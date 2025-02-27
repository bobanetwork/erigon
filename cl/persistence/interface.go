package persistence

import (
	"context"

	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon/cl/cltypes"
	"github.com/erigontech/erigon/cl/sentinel/peers"
)

type BlockSource interface {
	GetRange(ctx context.Context, tx kv.Tx, from uint64, count uint64) (*peers.PeeredObject[[]*cltypes.SignedBeaconBlock], error)
	PurgeRange(ctx context.Context, tx kv.Tx, from uint64, count uint64) error
	GetBlock(ctx context.Context, tx kv.Tx, slot uint64) (*peers.PeeredObject[*cltypes.SignedBeaconBlock], error)
}

type BeaconChainWriter interface {
	WriteBlock(ctx context.Context, tx kv.RwTx, block *cltypes.SignedBeaconBlock, canonical bool) error
}

type BeaconChainDatabase interface {
	BlockSource
	BeaconChainWriter
}
