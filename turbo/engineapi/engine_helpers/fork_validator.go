/*
   Copyright 2022 Erigon contributors
   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at
       http://www.apache.org/licenses/LICENSE-2.0
   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package engine_helpers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/erigontech/erigon/cl/phase1/core/state/lru"

	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/membatchwithdb"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon-lib/wrap"
	"github.com/erigontech/erigon/common/math"
	"github.com/erigontech/erigon/consensus"
	"github.com/erigontech/erigon/eth/stagedsync/stages"
	"github.com/erigontech/erigon/turbo/engineapi/engine_types"
	"github.com/erigontech/erigon/turbo/services"

	"github.com/erigontech/erigon/core/rawdb"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/turbo/shards"
)

// the maximum point from the current head, past which side forks are not validated anymore.
const maxForkDepth = 32 // 32 slots is the duration of an epoch thus there cannot be side forks in PoS deeper than 32 blocks from head.

type validatePayloadFunc func(wrap.TxContainer, *types.Header, *types.RawBody, uint64, []*types.Header, []*types.RawBody, *shards.Notifications) error

type ForkValidator struct {
	// current memory batch containing chain head that extend canonical fork.
	memoryDiff *membatchwithdb.MemoryDiff
	// notifications accumulated for the extending fork
	extendingForkNotifications *shards.Notifications
	// hash of chain head that extend canonical fork.
	extendingForkHeadHash libcommon.Hash
	extendingForkNumber   uint64
	// this is the function we use to perform payload validation.
	validatePayload validatePayloadFunc
	blockReader     services.FullBlockReader
	// this is the current point where we processed the chain so far.
	currentHeight uint64
	tmpDir        string
	// block hashes that are deemed valid
	validHashes *lru.Cache[libcommon.Hash, bool]

	ctx context.Context

	// we want fork validator to be thread safe so let
	lock sync.Mutex
}

func NewForkValidatorMock(currentHeight uint64) *ForkValidator {
	validHashes, err := lru.New[libcommon.Hash, bool]("validHashes", maxForkDepth*8)
	if err != nil {
		panic(err)
	}
	return &ForkValidator{
		currentHeight: currentHeight,
		validHashes:   validHashes,
	}
}

func NewForkValidator(ctx context.Context, currentHeight uint64, validatePayload validatePayloadFunc, tmpDir string, blockReader services.FullBlockReader) *ForkValidator {
	validHashes, err := lru.New[libcommon.Hash, bool]("validHashes", maxForkDepth*8)
	if err != nil {
		panic(err)
	}
	return &ForkValidator{
		validatePayload: validatePayload,
		currentHeight:   currentHeight,
		tmpDir:          tmpDir,
		blockReader:     blockReader,
		ctx:             ctx,
		validHashes:     validHashes,
	}
}

// ExtendingForkHeadHash return the fork head hash of the fork that extends the canonical chain.
func (fv *ForkValidator) ExtendingForkHeadHash() libcommon.Hash {
	fv.lock.Lock()
	defer fv.lock.Unlock()
	return fv.extendingForkHeadHash
}

// NotifyCurrentHeight is to be called at the end of the stage cycle and represent the last processed block.
func (fv *ForkValidator) NotifyCurrentHeight(currentHeight uint64) {
	fv.lock.Lock()
	defer fv.lock.Unlock()
	if fv.currentHeight == currentHeight {
		return
	}
	fv.currentHeight = currentHeight
	// If the head changed,e previous assumptions on head are incorrect now.
	fv.memoryDiff = nil
	fv.extendingForkNotifications = nil
	fv.extendingForkNumber = 0
	fv.extendingForkHeadHash = libcommon.Hash{}
}

// FlushExtendingFork flush the current extending fork if fcu chooses its head hash as the its forkchoice.
func (fv *ForkValidator) FlushExtendingFork(tx kv.RwTx, accumulator *shards.Accumulator) error {
	fv.lock.Lock()
	defer fv.lock.Unlock()
	// Flush changes to db.
	if err := fv.memoryDiff.Flush(tx); err != nil {
		return err
	}
	fv.extendingForkNotifications.Accumulator.CopyAndReset(accumulator)
	// Clean extending fork data
	fv.memoryDiff = nil
	fv.extendingForkHeadHash = libcommon.Hash{}
	fv.extendingForkNumber = 0
	fv.extendingForkNotifications = nil
	return nil
}

// ValidatePayload returns whether a payload is valid or invalid, or if cannot be determined, it will be accepted.
// if the payload extends the canonical chain, then we stack it in extendingFork without any unwind.
// if the payload is a fork then we unwind to the point where the fork meets the canonical chain, and there we check whether it is valid.
// if for any reason none of the actions above can be performed due to lack of information, we accept the payload and avoid validation.
func (fv *ForkValidator) ValidatePayload(tx kv.Tx, header *types.Header, body *types.RawBody, extendCanonical bool, logger log.Logger) (status engine_types.EngineStatus, latestValidHash libcommon.Hash, validationError error, criticalError error) {
	fv.lock.Lock()
	defer fv.lock.Unlock()
	if fv.validatePayload == nil {
		status = engine_types.AcceptedStatus
		return
	}

	// If the block is stored within the side fork it means it was already validated.
	if _, ok := fv.validHashes.Get(header.Hash()); ok {
		status = engine_types.ValidStatus
		latestValidHash = header.Hash()
		return
	}

	log.Debug("Execution ForkValidator.ValidatePayload", "extendCanonical", extendCanonical)
	if extendCanonical {
		extendingFork := membatchwithdb.NewMemoryBatch(tx, fv.tmpDir, logger)
		defer extendingFork.Close()
		var txc wrap.TxContainer
		txc.Tx = extendingFork

		fv.extendingForkNotifications = &shards.Notifications{
			Events:      shards.NewEvents(),
			Accumulator: shards.NewAccumulator(),
		}
		// Update fork head hash.
		fv.extendingForkHeadHash = header.Hash()
		fv.extendingForkNumber = header.Number.Uint64()
		status, latestValidHash, validationError, criticalError = fv.validateAndStorePayload(txc, header, body, 0, nil, nil, fv.extendingForkNotifications)
		if criticalError != nil {
			return
		}
		if validationError == nil {
			fv.memoryDiff, criticalError = extendingFork.Diff()
			if criticalError != nil {
				return
			}
		}
		return status, latestValidHash, validationError, criticalError
	}

	// if the block is not in range of maxForkDepth from head then we do not validate it.
	if math.AbsoluteDifference(fv.currentHeight, header.Number.Uint64()) > maxForkDepth {
		status = engine_types.AcceptedStatus
		return
	}
	// Let's assemble the side fork backwards
	var foundCanonical bool
	currentHash := header.ParentHash
	unwindPoint := header.Number.Uint64() - 1
	foundCanonical, criticalError = rawdb.IsCanonicalHash(tx, currentHash, unwindPoint)
	if criticalError != nil {
		return
	}

	logger.Debug("Execution ForkValidator.ValidatePayload", "foundCanonical", foundCanonical, "currentHash", currentHash, "unwindPoint", unwindPoint)

	var bodiesChain []*types.RawBody
	var headersChain []*types.Header
	for !foundCanonical {
		var (
			header *types.Header
			body   *types.Body
		)
		header, criticalError = fv.blockReader.Header(fv.ctx, tx, currentHash, unwindPoint)
		if criticalError != nil {
			return
		}
		if header == nil {
			// We miss some components so we did not check validity.
			status = engine_types.AcceptedStatus
			return
		}
		body, criticalError = fv.blockReader.BodyWithTransactions(fv.ctx, tx, currentHash, unwindPoint)
		if criticalError != nil {
			return
		}
		if body == nil {
			criticalError = fmt.Errorf("found chain gap in block body at hash %s, number %d", currentHash, unwindPoint)
			return
		}

		headersChain = append([]*types.Header{header}, headersChain...)
		bodiesChain = append([]*types.RawBody{body.RawBody()}, bodiesChain...)

		currentHash = header.ParentHash
		unwindPoint = header.Number.Uint64() - 1
		foundCanonical, criticalError = rawdb.IsCanonicalHash(tx, currentHash, unwindPoint)
		if criticalError != nil {
			return
		}
		logger.Debug("Execution ForkValidator.ValidatePayload", "foundCanonical", foundCanonical, "currentHash", currentHash, "unwindPoint", unwindPoint)
	}
	// Do not set an unwind point if we are already there.
	if unwindPoint == fv.currentHeight {
		unwindPoint = 0
	}
	batch := membatchwithdb.NewMemoryBatch(tx, fv.tmpDir, logger)
	defer batch.Rollback()
	var txc wrap.TxContainer
	txc.Tx = batch
	notifications := &shards.Notifications{
		Events:      shards.NewEvents(),
		Accumulator: shards.NewAccumulator(),
	}
	return fv.validateAndStorePayload(txc, header, body, unwindPoint, headersChain, bodiesChain, notifications)
}

// Clear wipes out current extending fork data, this method is called after fcu is called,
// because fcu decides what the head is and after the call is done all the non-chosen forks are
// to be considered obsolete.
func (fv *ForkValidator) clear() {
	fv.extendingForkHeadHash = libcommon.Hash{}
	fv.extendingForkNumber = 0
	fv.memoryDiff = nil
}

// Clear wipes out current extending fork data.
func (fv *ForkValidator) ClearWithUnwind(accumulator *shards.Accumulator, c shards.StateChangeConsumer) {
	fv.lock.Lock()
	defer fv.lock.Unlock()
	fv.clear()
}

// validateAndStorePayload validate and store a payload fork chain if such chain results valid.
func (fv *ForkValidator) validateAndStorePayload(txc wrap.TxContainer, header *types.Header, body *types.RawBody, unwindPoint uint64, headersChain []*types.Header, bodiesChain []*types.RawBody,
	notifications *shards.Notifications) (status engine_types.EngineStatus, latestValidHash libcommon.Hash, validationError error, criticalError error) {
	if err := fv.validatePayload(txc, header, body, unwindPoint, headersChain, bodiesChain, notifications); err != nil {
		if errors.Is(err, consensus.ErrInvalidBlock) {
			validationError = err
		} else {
			criticalError = fmt.Errorf("validateAndStorePayload: %w", err)
			return
		}
	}

	latestValidHash = header.Hash()
	if validationError != nil {
		var latestValidNumber uint64
		latestValidNumber, criticalError = stages.GetStageProgress(txc.Tx, stages.IntermediateHashes)
		if criticalError != nil {
			return
		}
		latestValidHash, criticalError = rawdb.ReadCanonicalHash(txc.Tx, latestValidNumber)
		if criticalError != nil {
			return
		}
		status = engine_types.InvalidStatus
		fv.memoryDiff = nil
		fv.extendingForkHeadHash = libcommon.Hash{}
		fv.extendingForkNumber = 0
		return
	}
	fv.validHashes.Add(header.Hash(), true)

	// If we do not have the body we can recover it from the batch.
	if body != nil {
		if _, criticalError = rawdb.WriteRawBodyIfNotExists(txc.Tx, header.Hash(), header.Number.Uint64(), body); criticalError != nil {
			return
		}
	}

	status = engine_types.ValidStatus
	return
}
