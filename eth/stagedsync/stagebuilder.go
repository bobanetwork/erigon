package stagedsync

import (
	"context"

	"github.com/erigontech/erigon-lib/log/v3"

	"github.com/erigontech/erigon-lib/gointerfaces/remote"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/wrap"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/eth/stagedsync/stages"
)

type ChainEventNotifier interface {
	OnNewHeader(newHeadersRlp [][]byte)
	OnNewPendingLogs(types.Logs)
	OnLogs([]*remote.SubscribeLogsReply)
	HasLogSubsriptions() bool
}

func MiningStages(
	ctx context.Context,
	createBlockCfg MiningCreateBlockCfg,
	borHeimdallCfg BorHeimdallCfg,
	execCfg MiningExecCfg,
	hashStateCfg HashStateCfg,
	trieCfg TrieCfg,
	finish MiningFinishCfg,
) []*Stage {
	return []*Stage{
		{
			ID:          stages.MiningCreateBlock,
			Description: "Mining: construct new block from tx pool",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *StageState, u Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnMiningCreateBlockStage(s, txc.Tx, createBlockCfg, ctx.Done(), logger)
			},
			Unwind: func(firstCycle bool, u *UnwindState, s *StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, u *PruneState, tx kv.RwTx, logger log.Logger) error { return nil },
		},
		{
			ID:          stages.MiningBorHeimdall,
			Description: "Download Bor-specific data from Heimdall",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *StageState, u Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				if badBlockUnwind {
					return nil
				}
				return MiningBorHeimdallForward(ctx, borHeimdallCfg, s, u, txc.Tx, logger)
			},
			Unwind: func(firstCycle bool, u *UnwindState, s *StageState, txc wrap.TxContainer, logger log.Logger) error {
				return BorHeimdallUnwind(u, ctx, s, txc.Tx, borHeimdallCfg)
			},
			Prune: func(firstCycle bool, p *PruneState, tx kv.RwTx, logger log.Logger) error {
				return BorHeimdallPrune(p, ctx, tx, borHeimdallCfg)
			},
		},
		{
			ID:          stages.MiningExecution,
			Description: "Mining: execute new block from tx pool",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *StageState, u Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnMiningExecStage(s, txc.Tx, execCfg, ctx.Done(), logger)
			},
			Unwind: func(firstCycle bool, u *UnwindState, s *StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, u *PruneState, tx kv.RwTx, logger log.Logger) error { return nil },
		},
		{
			ID:          stages.HashState,
			Description: "Hash the key in the state",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *StageState, u Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnHashStateStage(s, txc.Tx, hashStateCfg, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *UnwindState, s *StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, u *PruneState, tx kv.RwTx, logger log.Logger) error { return nil },
		},
		{
			ID:          stages.IntermediateHashes,
			Description: "Generate intermediate hashes and computing state root",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *StageState, u Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				stateRoot, err := SpawnIntermediateHashesStage(s, u, txc.Tx, trieCfg, ctx, logger)
				if err != nil {
					return err
				}
				createBlockCfg.miner.MiningBlock.Header.Root = stateRoot
				return nil
			},
			Unwind: func(firstCycle bool, u *UnwindState, s *StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, u *PruneState, tx kv.RwTx, logger log.Logger) error { return nil },
		},
		{
			ID:          stages.MiningFinish,
			Description: "Mining: create and propagate valid block",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *StageState, u Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnMiningFinishStage(s, txc.Tx, finish, ctx.Done(), logger)
			},
			Unwind: func(firstCycle bool, u *UnwindState, s *StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, u *PruneState, tx kv.RwTx, logger log.Logger) error { return nil },
		},
	}
}
