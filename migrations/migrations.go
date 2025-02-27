package migrations

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/erigontech/erigon-lib/common"

	"github.com/erigontech/erigon-lib/common/datadir"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon/core/rawdb"
	"github.com/erigontech/erigon/eth/stagedsync/stages"
	"github.com/ugorji/go/codec"
)

// migrations apply sequentially in order of this array, skips applied migrations
// it allows - don't worry about merge conflicts and use switch branches
// see also dbutils.Migrations - it stores context in which each transaction was exectured - useful for bug-reports
//
// Idempotency is expected
// Best practices to achieve Idempotency:
//   - in dbutils/bucket.go add suffix for existing bucket variable, create new bucket with same variable name.
//     Example:
//   - SyncStageProgress = []byte("SSP1")
//   - SyncStageProgressOld1 = []byte("SSP1")
//   - SyncStageProgress = []byte("SSP2")
//   - in the beginning of migration: check that old bucket exists, clear new bucket
//   - in the end:drop old bucket (not in defer!).
//   - if you need migrate multiple buckets - create separate migration for each bucket
//   - write test - and check that it's safe to apply same migration twice
var migrations = map[kv.Label][]Migration{
	kv.ChainDB: {
		dbSchemaVersion5,
		TxsBeginEnd,
		TxsV3,
		ProhibitNewDownloadsLock,
		ProhibitNewDownloadsLock2,
	},
	kv.TxPoolDB: {},
	kv.SentryDB: {},
}

type Callback func(tx kv.RwTx, progress []byte, isDone bool) error
type Migration struct {
	Name string
	Up   func(db kv.RwDB, dirs datadir.Dirs, progress []byte, BeforeCommit Callback, logger log.Logger) error
}

var (
	ErrMigrationNonUniqueName   = fmt.Errorf("please provide unique migration name")
	ErrMigrationCommitNotCalled = fmt.Errorf("migration before-commit function was not called")
	ErrMigrationETLFilesDeleted = fmt.Errorf(
		"db migration progress was interrupted after extraction step and ETL files was deleted, please contact development team for help or re-sync from scratch",
	)
)

func NewMigrator(label kv.Label) *Migrator {
	return &Migrator{
		Migrations: migrations[label],
	}
}

type Migrator struct {
	Migrations []Migration
}

func AppliedMigrations(tx kv.Tx, withPayload bool) (map[string][]byte, error) {
	applied := map[string][]byte{}
	err := tx.ForEach(kv.Migrations, nil, func(k []byte, v []byte) error {
		if bytes.HasPrefix(k, []byte("_progress_")) {
			return nil
		}
		if withPayload {
			applied[string(common.CopyBytes(k))] = common.CopyBytes(v)
		} else {
			applied[string(common.CopyBytes(k))] = []byte{}
		}
		return nil
	})
	return applied, err
}

func (m *Migrator) HasPendingMigrations(db kv.RwDB) (bool, error) {
	var has bool
	if err := db.View(context.Background(), func(tx kv.Tx) error {
		pending, err := m.PendingMigrations(tx)
		if err != nil {
			return err
		}
		has = len(pending) > 0
		return nil
	}); err != nil {
		return false, err
	}
	return has, nil
}

func (m *Migrator) PendingMigrations(tx kv.Tx) ([]Migration, error) {
	applied, err := AppliedMigrations(tx, false)
	if err != nil {
		return nil, err
	}

	counter := 0
	for i := range m.Migrations {
		v := m.Migrations[i]
		if _, ok := applied[v.Name]; ok {
			continue
		}
		counter++
	}

	pending := make([]Migration, 0, counter)
	for i := range m.Migrations {
		v := m.Migrations[i]
		if _, ok := applied[v.Name]; ok {
			continue
		}
		pending = append(pending, v)
	}
	return pending, nil
}

func (m *Migrator) VerifyVersion(db kv.RwDB) error {
	if err := db.View(context.Background(), func(tx kv.Tx) error {
		major, minor, _, ok, err := rawdb.ReadDBSchemaVersion(tx)
		if err != nil {
			return fmt.Errorf("reading DB schema version: %w", err)
		}
		if ok {
			if major > kv.DBSchemaVersion.Major {
				return fmt.Errorf("cannot downgrade major DB version from %d to %d", major, kv.DBSchemaVersion.Major)
			} else if major == kv.DBSchemaVersion.Major {
				if minor > kv.DBSchemaVersion.Minor {
					return fmt.Errorf("cannot downgrade minor DB version from %d.%d to %d.%d", major, minor, kv.DBSchemaVersion.Major, kv.DBSchemaVersion.Major)
				}
			} else {
				// major < kv.DBSchemaVersion.Major
				if kv.DBSchemaVersion.Major-major > 1 {
					return fmt.Errorf("cannot upgrade major DB version for more than 1 version from %d to %d, use integration tool if you know what you are doing", major, kv.DBSchemaVersion.Major)
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("migrator.VerifyVersion: %w", err)
	}

	return nil
}

func (m *Migrator) Apply(db kv.RwDB, dataDir string, logger log.Logger) error {
	if len(m.Migrations) == 0 {
		return nil
	}
	dirs := datadir.New(dataDir)

	var applied map[string][]byte
	if err := db.View(context.Background(), func(tx kv.Tx) error {
		var err error
		applied, err = AppliedMigrations(tx, false)
		if err != nil {
			return fmt.Errorf("reading applied migrations: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := m.VerifyVersion(db); err != nil {
		return fmt.Errorf("migrator.Apply: %w", err)
	}

	// migration names must be unique, protection against people's mistake
	uniqueNameCheck := map[string]bool{}
	for i := range m.Migrations {
		_, ok := uniqueNameCheck[m.Migrations[i].Name]
		if ok {
			return fmt.Errorf("%w, duplicate: %s", ErrMigrationNonUniqueName, m.Migrations[i].Name)
		}
		uniqueNameCheck[m.Migrations[i].Name] = true
	}

	for i := range m.Migrations {
		v := m.Migrations[i]
		if _, ok := applied[v.Name]; ok {
			continue
		}

		callbackCalled := false // commit function must be called if no error, protection against people's mistake

		logger.Info("Apply migration", "name", v.Name)
		var progress []byte
		if err := db.View(context.Background(), func(tx kv.Tx) (err error) {
			progress, err = tx.GetOne(kv.Migrations, []byte("_progress_"+v.Name))
			return err
		}); err != nil {
			return fmt.Errorf("migrator.Apply: %w", err)
		}

		dirs.Tmp = filepath.Join(dirs.DataDir, "migrations", v.Name)
		if err := v.Up(db, dirs, progress, func(tx kv.RwTx, key []byte, isDone bool) error {
			if !isDone {
				if key != nil {
					if err := tx.Put(kv.Migrations, []byte("_progress_"+v.Name), key); err != nil {
						return err
					}
				}
				return nil
			}
			callbackCalled = true

			stagesProgress, err := MarshalMigrationPayload(tx)
			if err != nil {
				return err
			}
			err = tx.Put(kv.Migrations, []byte(v.Name), stagesProgress)
			if err != nil {
				return err
			}

			err = tx.Delete(kv.Migrations, []byte("_progress_"+v.Name))
			if err != nil {
				return err
			}

			return nil
		}, logger); err != nil {
			return fmt.Errorf("migrator.Apply.Up: %s, %w", v.Name, err)
		}

		if !callbackCalled {
			return fmt.Errorf("%w: %s", ErrMigrationCommitNotCalled, v.Name)
		}
		logger.Info("Applied migration", "name", v.Name)
	}
	if err := db.Update(context.Background(), func(tx kv.RwTx) error {
		return rawdb.WriteDBSchemaVersion(tx)
	}); err != nil {
		return fmt.Errorf("migrator.Apply: %w", err)
	}
	logger.Info(
		"Updated DB schema to",
		"version",
		fmt.Sprintf(
			"%d.%d.%d",
			kv.DBSchemaVersion.Major,
			kv.DBSchemaVersion.Minor,
			kv.DBSchemaVersion.Patch,
		),
	)
	return nil
}

func MarshalMigrationPayload(db kv.Getter) ([]byte, error) {
	s := map[string][]byte{}

	buf := bytes.NewBuffer(nil)
	encoder := codec.NewEncoder(buf, &codec.CborHandle{})

	for _, stage := range stages.AllStages {
		v, err := db.GetOne(kv.SyncStageProgress, []byte(stage))
		if err != nil {
			return nil, err
		}
		if len(v) > 0 {
			s[string(stage)] = common.CopyBytes(v)
		}
	}

	if err := encoder.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func UnmarshalMigrationPayload(data []byte) (map[string][]byte, error) {
	s := map[string][]byte{}

	if err := codec.NewDecoder(bytes.NewReader(data), &codec.CborHandle{}).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}
