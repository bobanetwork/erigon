package jsonrpc

import (
	"context"
	"math/big"
	"testing"

	"github.com/erigontech/erigon-lib/common/hexutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/gointerfaces/txpool"
	"github.com/erigontech/erigon-lib/kv/kvcache"

	"github.com/erigontech/erigon-lib/log/v3"

	"github.com/erigontech/erigon/cmd/rpcdaemon/rpcdaemontest"
	"github.com/erigontech/erigon/common/u256"
	"github.com/erigontech/erigon/core/rawdb"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/crypto"
	"github.com/erigontech/erigon/params"
	"github.com/erigontech/erigon/rlp"
	"github.com/erigontech/erigon/rpc"
	"github.com/erigontech/erigon/rpc/rpccfg"
	"github.com/erigontech/erigon/turbo/rpchelper"
	"github.com/erigontech/erigon/turbo/stages/mock"
)

// Gets the latest block number with the latest tag
func TestGetBlockByNumberWithLatestTag(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	b, err := api.GetBlockByNumber(context.Background(), rpc.LatestBlockNumber, false)
	expected := common.HexToHash("0x5883164d4100b95e1d8e931b8b9574586a1dea7507941e6ad3c1e3a2591485fd")
	if err != nil {
		t.Errorf("error getting block number with latest tag: %s", err)
	}
	assert.Equal(t, expected, b["hash"])
}

func TestGetBlockByNumberWithLatestTag_WithHeadHashInDb(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	latestBlockHash := common.HexToHash("0x6804117de2f3e6ee32953e78ced1db7b20214e0d8c745a03b8fecf7cc8ee76ef")
	latestBlock, err := m.BlockReader.BlockByHash(ctx, tx, latestBlockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("couldn't retrieve latest block")
	}
	rawdb.WriteHeaderNumber(tx, latestBlockHash, latestBlock.NonceU64())
	rawdb.WriteForkchoiceHead(tx, latestBlockHash)
	if safedHeadBlock := rawdb.ReadForkchoiceHead(tx); safedHeadBlock == (common.Hash{}) {
		tx.Rollback()
		t.Error("didn't find forkchoice head hash")
	}
	tx.Commit()

	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	block, err := api.GetBlockByNumber(ctx, rpc.LatestBlockNumber, false)
	if err != nil {
		t.Errorf("error retrieving block by number: %s", err)
	}
	expectedHash := common.HexToHash("0x71b89b6ca7b65debfd2fbb01e4f07de7bba343e6617559fa81df19b605f84662")
	assert.Equal(t, expectedHash, block["hash"])
}

func TestGetBlockByNumberWithPendingTag(t *testing.T) {
	m := mock.MockWithTxPool(t)
	agg := m.HistoryV3Components()
	stateCache := kvcache.New(kvcache.DefaultCoherentConfig)

	ctx, conn := rpcdaemontest.CreateTestGrpcConn(t, m)
	txPool := txpool.NewTxpoolClient(conn)
	ff := rpchelper.New(ctx, rpchelper.DefaultFiltersConfig, nil, txPool, txpool.NewMiningClient(conn), func() {}, m.Log)

	expected := 1
	header := &types.Header{
		Number: big.NewInt(int64(expected)),
	}

	rlpBlock, err := rlp.EncodeToBytes(types.NewBlockWithHeader(header))
	if err != nil {
		t.Errorf("failed encoding the block: %s", err)
	}
	ff.HandlePendingBlock(&txpool.OnPendingBlockReply{
		RplBlock: rlpBlock,
	})

	api := NewEthAPI(NewBaseApi(ff, stateCache, m.BlockReader, agg, false, rpccfg.DefaultEvmCallTimeout, m.Engine, m.Dirs, nil, nil), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	b, err := api.GetBlockByNumber(context.Background(), rpc.PendingBlockNumber, false)
	if err != nil {
		t.Errorf("error getting block number with pending tag: %s", err)
	}
	assert.Equal(t, (*hexutil.Big)(big.NewInt(int64(expected))), b["number"])
}

func TestGetBlockByNumber_WithFinalizedTag_NoFinalizedBlockInDb(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	if _, err := api.GetBlockByNumber(ctx, rpc.FinalizedBlockNumber, false); err != nil {
		assert.ErrorIs(t, rpchelper.UnknownBlockError, err)
	}
}

func TestGetBlockByNumber_WithFinalizedTag_WithFinalizedBlockInDb(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	latestBlockHash := common.HexToHash("0x6804117de2f3e6ee32953e78ced1db7b20214e0d8c745a03b8fecf7cc8ee76ef")
	latestBlock, err := m.BlockReader.BlockByHash(ctx, tx, latestBlockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("couldn't retrieve latest block")
	}
	rawdb.WriteHeaderNumber(tx, latestBlockHash, latestBlock.NonceU64())
	rawdb.WriteForkchoiceFinalized(tx, latestBlockHash)
	if safedFinalizedBlock := rawdb.ReadForkchoiceFinalized(tx); safedFinalizedBlock == (common.Hash{}) {
		tx.Rollback()
		t.Error("didn't find forkchoice finalized hash")
	}
	tx.Commit()

	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	block, err := api.GetBlockByNumber(ctx, rpc.FinalizedBlockNumber, false)
	if err != nil {
		t.Errorf("error retrieving block by number: %s", err)
	}
	expectedHash := common.HexToHash("0x71b89b6ca7b65debfd2fbb01e4f07de7bba343e6617559fa81df19b605f84662")
	assert.Equal(t, expectedHash, block["hash"])
}

func TestGetBlockByNumber_WithSafeTag_NoSafeBlockInDb(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	if _, err := api.GetBlockByNumber(ctx, rpc.SafeBlockNumber, false); err != nil {
		assert.ErrorIs(t, rpchelper.UnknownBlockError, err)
	}
}

func TestGetBlockByNumber_WithSafeTag_WithSafeBlockInDb(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	latestBlockHash := common.HexToHash("0x6804117de2f3e6ee32953e78ced1db7b20214e0d8c745a03b8fecf7cc8ee76ef")
	latestBlock, err := m.BlockReader.BlockByHash(ctx, tx, latestBlockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("couldn't retrieve latest block")
	}
	rawdb.WriteHeaderNumber(tx, latestBlockHash, latestBlock.NonceU64())
	rawdb.WriteForkchoiceSafe(tx, latestBlockHash)
	if safedSafeBlock := rawdb.ReadForkchoiceSafe(tx); safedSafeBlock == (common.Hash{}) {
		tx.Rollback()
		t.Error("didn't find forkchoice safe block hash")
	}
	tx.Commit()

	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	block, err := api.GetBlockByNumber(ctx, rpc.SafeBlockNumber, false)
	if err != nil {
		t.Errorf("error retrieving block by number: %s", err)
	}
	expectedHash := common.HexToHash("0x71b89b6ca7b65debfd2fbb01e4f07de7bba343e6617559fa81df19b605f84662")
	assert.Equal(t, expectedHash, block["hash"])
}

func TestGetBlockTransactionCountByHash(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()

	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	blockHash := common.HexToHash("0x6804117de2f3e6ee32953e78ced1db7b20214e0d8c745a03b8fecf7cc8ee76ef")

	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	header, err := rawdb.ReadHeaderByHash(tx, blockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("failed reading block by hash: %s", err)
	}
	bodyWithTx, err := m.BlockReader.BodyWithTransactions(ctx, tx, blockHash, header.Number.Uint64())
	if err != nil {
		tx.Rollback()
		t.Errorf("failed getting body with transactions: %s", err)
	}
	tx.Rollback()

	expectedAmount := hexutil.Uint(len(bodyWithTx.Transactions))

	txAmount, err := api.GetBlockTransactionCountByHash(ctx, blockHash)
	if err != nil {
		t.Errorf("failed getting the transaction count, err=%s", err)
	}

	assert.Equal(t, expectedAmount, *txAmount)
}

func TestGetBlockTransactionCountByHash_ZeroTx(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	blockHash := common.HexToHash("0x5883164d4100b95e1d8e931b8b9574586a1dea7507941e6ad3c1e3a2591485fd")

	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	header, err := rawdb.ReadHeaderByHash(tx, blockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("failed reading block by hash: %s", err)
	}
	bodyWithTx, err := m.BlockReader.BodyWithTransactions(ctx, tx, blockHash, header.Number.Uint64())
	if err != nil {
		tx.Rollback()
		t.Errorf("failed getting body with transactions: %s", err)
	}
	tx.Rollback()

	expectedAmount := hexutil.Uint(len(bodyWithTx.Transactions))

	txAmount, err := api.GetBlockTransactionCountByHash(ctx, blockHash)
	if err != nil {
		t.Errorf("failed getting the transaction count, err=%s", err)
	}

	assert.Equal(t, expectedAmount, *txAmount)
}

func TestGetBlockTransactionCountByNumber(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	blockHash := common.HexToHash("0x6804117de2f3e6ee32953e78ced1db7b20214e0d8c745a03b8fecf7cc8ee76ef")

	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	header, err := rawdb.ReadHeaderByHash(tx, blockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("failed reading block by hash: %s", err)
	}
	bodyWithTx, err := m.BlockReader.BodyWithTransactions(ctx, tx, blockHash, header.Number.Uint64())
	if err != nil {
		tx.Rollback()
		t.Errorf("failed getting body with transactions: %s", err)
	}
	tx.Rollback()

	expectedAmount := hexutil.Uint(len(bodyWithTx.Transactions))

	txAmount, err := api.GetBlockTransactionCountByNumber(ctx, rpc.BlockNumber(header.Number.Uint64()))
	if err != nil {
		t.Errorf("failed getting the transaction count, err=%s", err)
	}

	assert.Equal(t, expectedAmount, *txAmount)
}

func TestGetBlockTransactionCountByNumber_ZeroTx(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	ctx := context.Background()
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())

	blockHash := common.HexToHash("0x5883164d4100b95e1d8e931b8b9574586a1dea7507941e6ad3c1e3a2591485fd")

	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}
	header, err := rawdb.ReadHeaderByHash(tx, blockHash)
	if err != nil {
		tx.Rollback()
		t.Errorf("failed reading block by hash: %s", err)
	}
	bodyWithTx, err := m.BlockReader.BodyWithTransactions(ctx, tx, blockHash, header.Number.Uint64())
	if err != nil {
		tx.Rollback()
		t.Errorf("failed getting body with transactions: %s", err)
	}
	tx.Rollback()

	expectedAmount := hexutil.Uint(len(bodyWithTx.Transactions))

	txAmount, err := api.GetBlockTransactionCountByNumber(ctx, rpc.BlockNumber(header.Number.Uint64()))
	if err != nil {
		t.Errorf("failed getting the transaction count, err=%s", err)
	}

	assert.Equal(t, expectedAmount, *txAmount)
}

func TestGetBadBlocks(t *testing.T) {
	m, _, _ := rpcdaemontest.CreateTestSentry(t)
	api := NewEthAPI(newBaseApiForTest(m), m.DB, nil, nil, nil, 5000000, 1e18, 100_000, false, 100_000, 128, log.New())
	ctx := context.Background()

	require := require.New(t)
	var testKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddr := crypto.PubkeyToAddress(testKey.PublicKey)

	mustSign := func(tx types.Transaction, s types.Signer) types.Transaction {
		r, err := types.SignTx(tx, s, testKey)
		require.NoError(err)
		return r
	}

	tx, err := m.DB.BeginRw(ctx)
	if err != nil {
		t.Errorf("could not begin read write transaction: %s", err)
	}

	putBlock := func(number uint64) common.Hash {
		// prepare db so it works with our test
		signer1 := types.MakeSigner(params.MainnetChainConfig, number, number-1)
		body := &types.Body{
			Transactions: []types.Transaction{
				mustSign(types.NewTransaction(number, testAddr, u256.Num1, 1, u256.Num1, nil), *signer1),
				mustSign(types.NewTransaction(number+1, testAddr, u256.Num1, 2, u256.Num1, nil), *signer1),
			},
			Uncles: []*types.Header{{Extra: []byte("test header")}},
		}

		header := &types.Header{Number: big.NewInt(int64(number))}
		require.NoError(rawdb.WriteCanonicalHash(tx, header.Hash(), number))
		require.NoError(rawdb.WriteHeader(tx, header))
		require.NoError(rawdb.WriteBody(tx, header.Hash(), number, body))

		return header.Hash()
	}

	number := *rawdb.ReadCurrentBlockNumber(tx)

	// put some blocks
	i := number
	for i <= number+6 {
		putBlock(i)
		i++
	}
	hash1 := putBlock(i)
	hash2 := putBlock(i + 1)
	hash3 := putBlock(i + 2)
	hash4 := putBlock(i + 3)
	require.NoError(rawdb.TruncateCanonicalHash(tx, i, true)) // trim since i

	tx.Commit()

	data, err := api.GetBadBlocks(ctx)
	require.NoError(err)

	require.Len(data, 4)
	require.Equal(data[0]["hash"], hash4)
	require.Equal(data[1]["hash"], hash3)
	require.Equal(data[2]["hash"], hash2)
	require.Equal(data[3]["hash"], hash1)
}
