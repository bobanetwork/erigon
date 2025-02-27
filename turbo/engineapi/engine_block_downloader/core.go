package engine_block_downloader

import (
	"context"

	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/gointerfaces/execution"
	"github.com/erigontech/erigon-lib/kv/mdbx"
	"github.com/erigontech/erigon-lib/kv/membatchwithdb"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/turbo/stages/headerdownload"
)

// download is the process that reverse download a specific block hash.
func (e *EngineBlockDownloader) download(ctx context.Context, hashToDownload libcommon.Hash, requestId int, block *types.Block) {
	/* Start download process*/
	// First we schedule the headers download process
	if !e.scheduleHeadersDownload(requestId, hashToDownload, 0) {
		e.logger.Warn("[EngineBlockDownloader] could not begin header download")
		// could it be scheduled? if not nevermind.
		e.status.Store(headerdownload.Idle)
		return
	}
	// see the outcome of header download
	headersStatus := e.waitForEndOfHeadersDownload()

	if headersStatus != headerdownload.Synced {
		// Could not sync. Set to idle
		e.logger.Warn("[EngineBlockDownloader] Header download did not yield success")
		e.status.Store(headerdownload.Idle)
		return
	}
	e.hd.SetPosStatus(headerdownload.Idle)

	tx, err := e.db.BeginRo(ctx)
	if err != nil {
		e.logger.Warn("[EngineBlockDownloader] Could not begin tx", "err", err)
		e.status.Store(headerdownload.Idle)
		return
	}
	defer tx.Rollback()

	tmpDb, err := mdbx.NewTemporaryMdbx(ctx, e.tmpdir)
	if err != nil {
		e.logger.Warn("[EngineBlockDownloader] Could create temporary mdbx", "err", err)
		e.status.Store(headerdownload.Idle)
		return
	}
	defer tmpDb.Close()
	tmpTx, err := tmpDb.BeginRw(ctx)
	if err != nil {
		e.logger.Warn("[EngineBlockDownloader] Could create temporary mdbx", "err", err)
		e.status.Store(headerdownload.Idle)
		return
	}
	defer tmpTx.Rollback()

	memoryMutation := membatchwithdb.NewMemoryBatchWithCustomDB(tx, tmpDb, tmpTx, e.tmpdir)
	defer memoryMutation.Rollback()

	startBlock, endBlock, startHash, err := e.loadDownloadedHeaders(memoryMutation)
	if err != nil {
		e.logger.Warn("[EngineBlockDownloader] Could load headers", "err", err)
		e.status.Store(headerdownload.Idle)
		return
	}

	// bodiesCollector := etl.NewCollector("EngineBlockDownloader", e.tmpdir, etl.NewSortableBuffer(etl.BufferOptimalSize), e.logger)
	if err := e.downloadAndLoadBodiesSyncronously(ctx, memoryMutation, startBlock, endBlock); err != nil {
		e.logger.Warn("[EngineBlockDownloader] Could not download bodies", "err", err)
		e.status.Store(headerdownload.Idle)
		return
	}
	tx.Rollback() // Discard the original db tx
	if err := e.insertHeadersAndBodies(ctx, tmpTx, startBlock, startHash, endBlock); err != nil {
		e.logger.Warn("[EngineBlockDownloader] Could not insert headers and bodies", "err", err)
		e.status.Store(headerdownload.Idle)
		return
	}
	e.logger.Info("[EngineBlockDownloader] Finished downloading blocks", "from", startBlock-1, "to", endBlock)
	if block == nil {
		e.status.Store(headerdownload.Idle)
		return
	}
	// Can fail, not an issue in this case.
	e.chainRW.InsertBlockAndWait(ctx, block)
	// Lastly attempt verification
	status, _, latestValidHash, err := e.chainRW.ValidateChain(ctx, block.Hash(), block.NumberU64())
	if err != nil {
		e.logger.Warn("[EngineBlockDownloader] block verification failed", "reason", err)
		e.status.Store(headerdownload.Idle)
		return
	}
	if status == execution.ExecutionStatus_TooFarAway || status == execution.ExecutionStatus_Busy {
		e.logger.Info("[EngineBlockDownloader] block verification skipped")
		e.status.Store(headerdownload.Synced)
		return
	}
	if status == execution.ExecutionStatus_BadBlock {
		e.logger.Warn("[EngineBlockDownloader] block segments downloaded are invalid")
		e.status.Store(headerdownload.Idle)
		e.hd.ReportBadHeaderPoS(block.Hash(), latestValidHash)
		return
	}
	e.logger.Info("[EngineBlockDownloader] blocks verification successful")
	e.status.Store(headerdownload.Synced)

}

// StartDownloading triggers the download process and returns true if the process started or false if it could not.
// blockTip is optional and should be the block tip of the download request. which will be inserted at the end of the procedure if specified.
func (e *EngineBlockDownloader) StartDownloading(ctx context.Context, requestId int, hashToDownload libcommon.Hash, blockTip *types.Block) bool {
	e.lock.Lock()
	defer e.lock.Unlock()
	if e.status.Load() == headerdownload.Syncing {
		return false
	}
	e.status.Store(headerdownload.Syncing)
	go e.download(e.bacgroundCtx, hashToDownload, requestId, blockTip)
	return true
}

func (e *EngineBlockDownloader) Status() headerdownload.SyncStatus {
	return headerdownload.SyncStatus(e.status.Load().(int))
}
