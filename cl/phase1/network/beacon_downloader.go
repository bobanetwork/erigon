package network

import (
	"sync"
	"sync/atomic"
	"time"

	libcommon "github.com/erigontech/erigon-lib/common"
	"golang.org/x/net/context"

	"github.com/erigontech/erigon/cl/cltypes"
	"github.com/erigontech/erigon/cl/rpc"
)

// Input: the currently highest slot processed and the list of blocks we want to know process
// Output: the new last new highest slot processed and an error possibly?
type ProcessFn func(
	highestSlotProcessed uint64,
	highestBlockRootProcessed libcommon.Hash,
	blocks []*cltypes.SignedBeaconBlock) (
	newHighestSlotProcessed uint64,
	newHighestBlockRootProcessed libcommon.Hash,
	err error)

type ForwardBeaconDownloader struct {
	ctx                       context.Context
	highestSlotProcessed      uint64
	highestBlockRootProcessed libcommon.Hash
	rpc                       *rpc.BeaconRpcP2P
	process                   ProcessFn

	mu sync.Mutex
}

func NewForwardBeaconDownloader(ctx context.Context, rpc *rpc.BeaconRpcP2P) *ForwardBeaconDownloader {
	return &ForwardBeaconDownloader{
		ctx: ctx,
		rpc: rpc,
	}
}

// SetProcessFunction sets the function used to process segments.
func (f *ForwardBeaconDownloader) SetProcessFunction(fn ProcessFn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.process = fn
}

// SetHighestProcessedSlot sets the highest processed slot so far.
func (f *ForwardBeaconDownloader) SetHighestProcessedSlot(highestSlotProcessed uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.highestSlotProcessed = highestSlotProcessed
}

// SetHighestProcessedRoot sets the highest processed block root so far.
func (f *ForwardBeaconDownloader) SetHighestProcessedRoot(root libcommon.Hash) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.highestBlockRootProcessed = root
}

// HighestProcessedRoot returns the highest processed block root so far.
func (f *ForwardBeaconDownloader) HighestProcessedRoot() libcommon.Hash {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.highestBlockRootProcessed
}

type peerAndBlocks struct {
	peerId string
	blocks []*cltypes.SignedBeaconBlock
}

func (f *ForwardBeaconDownloader) RequestMore(ctx context.Context) {
	count := uint64(16)
	var atomicResp atomic.Value
	atomicResp.Store(peerAndBlocks{})
	reqInterval := time.NewTicker(300 * time.Millisecond)
	defer reqInterval.Stop()
Loop:
	for {
		select {
		case <-reqInterval.C:
			go func() {
				if len(atomicResp.Load().(peerAndBlocks).blocks) > 0 {
					return
				}
				// this is so we do not get stuck on a side-fork
				responses, peerId, err := f.rpc.SendBeaconBlocksByRangeReq(ctx, f.highestSlotProcessed-2, count)

				if err != nil {
					return
				}
				if responses == nil {
					return
				}
				if len(responses) == 0 {
					f.rpc.BanPeer(peerId)
					return
				}
				if len(atomicResp.Load().(peerAndBlocks).blocks) > 0 {
					return
				}
				atomicResp.Store(peerAndBlocks{peerId, responses})
			}()
		case <-ctx.Done():
			return
		default:
			if len(atomicResp.Load().(peerAndBlocks).blocks) > 0 {
				break Loop
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	var highestBlockRootProcessed libcommon.Hash
	var highestSlotProcessed uint64
	var err error
	blocks := atomicResp.Load().(peerAndBlocks).blocks
	pid := atomicResp.Load().(peerAndBlocks).peerId
	if highestSlotProcessed, highestBlockRootProcessed, err = f.process(f.highestSlotProcessed, f.highestBlockRootProcessed, blocks); err != nil {
		f.rpc.BanPeer(pid)
		return
	}
	f.highestSlotProcessed = highestSlotProcessed
	f.highestBlockRootProcessed = highestBlockRootProcessed
}

// GetHighestProcessedSlot retrieve the highest processed slot we accumulated.
func (f *ForwardBeaconDownloader) GetHighestProcessedSlot() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.highestSlotProcessed
}

func (f *ForwardBeaconDownloader) Peers() (uint64, error) {
	return f.rpc.Peers()
}
