package jsonrpc

import (
	"context"
	"fmt"

	"github.com/erigontech/erigon-lib/common"

	"github.com/erigontech/erigon/rpc"
	"github.com/erigontech/erigon/turbo/rpchelper"
)

func (api *OtterscanAPIImpl) HasCode(ctx context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (bool, error) {
	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return false, fmt.Errorf("hasCode cannot open tx: %w", err)
	}
	defer tx.Rollback()

	blockNumber, _, _, err := rpchelper.GetBlockNumber(blockNrOrHash, tx, api.filters)
	if err != nil {
		return false, err
	}
	chainConfig, err := api.chainConfig(ctx, tx)
	if err != nil {
		return false, err
	}

	reader, err := rpchelper.CreateHistoryStateReader(tx, blockNumber, 0, api.historyV3(tx), chainConfig.ChainName)
	if err != nil {
		return false, err
	}
	acc, err := reader.ReadAccountData(address)
	if acc == nil || err != nil {
		return false, err
	}
	return !acc.IsEmptyCodeHash(), nil
}
