package jsonrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erigontech/erigon-lib/common"

	"github.com/erigontech/erigon-lib/log/v3"

	"github.com/erigontech/erigon/cmd/rpcdaemon/rpcdaemontest"
	"github.com/erigontech/erigon/rpc"
)

// TestNotFoundMustReturnNil - next methods - when record not found in db - must return nil instead of error
// see https://github.com/erigontech/erigon/issues/1645
func TestNotFoundMustReturnNil(t *testing.T) {
	require := require.New(t)
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	api := NewEthAPI(newBaseApiForTest(m),
		m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	ctx := context.Background()

	a, err := api.GetTransactionByBlockNumberAndIndex(ctx, 10_000, 1)
	require.Nil(a)
	require.Nil(err)

	b, err := api.GetTransactionByBlockHashAndIndex(ctx, common.Hash{}, 1)
	require.Nil(b)
	require.Nil(err)

	c, err := api.GetTransactionByBlockNumberAndIndex(ctx, 10_000, 1)
	require.Nil(c)
	require.Nil(err)

	d, err := api.GetTransactionReceipt(ctx, common.Hash{})
	require.Nil(d)
	require.Nil(err)

	e, err := api.GetBlockByHash(ctx, rpc.BlockNumberOrHashWithHash(common.Hash{}, true), false)
	require.Nil(e)
	require.Nil(err)

	f, err := api.GetBlockByNumber(ctx, 10_000, false)
	require.Nil(f)
	require.Nil(err)

	g, err := api.GetUncleByBlockHashAndIndex(ctx, common.Hash{}, 1)
	require.Nil(g)
	require.Nil(err)

	h, err := api.GetUncleByBlockNumberAndIndex(ctx, 10_000, 1)
	require.Nil(h)
	require.Nil(err)

	j, err := api.GetBlockTransactionCountByNumber(ctx, 10_000)
	require.Nil(j)
	require.Nil(err)

	k, err := api.GetBadBlocks(ctx)
	require.Nil(k)
	require.Nil(err)
}
