package jsonrpc

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/holiman/uint256"
	"golang.org/x/sync/errgroup"

	"github.com/erigontech/erigon-lib/chain"
	"github.com/erigontech/erigon-lib/common"
	hexutil2 "github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/common/hexutility"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/iter"
	"github.com/erigontech/erigon-lib/kv/order"
	"github.com/erigontech/erigon-lib/kv/rawdbv3"
	"github.com/erigontech/erigon/consensus"
	"github.com/erigontech/erigon/core"
	"github.com/erigontech/erigon/core/rawdb"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/core/vm"
	"github.com/erigontech/erigon/core/vm/evmtypes"
	"github.com/erigontech/erigon/eth/ethutils"
	"github.com/erigontech/erigon/rpc"
	"github.com/erigontech/erigon/turbo/adapter/ethapi"
	"github.com/erigontech/erigon/turbo/rpchelper"
	"github.com/erigontech/erigon/turbo/transactions"
)

// API_LEVEL Must be incremented every time new additions are made
const API_LEVEL = 8

type TransactionsWithReceipts struct {
	Txs       []*RPCTransaction        `json:"txs"`
	Receipts  []map[string]interface{} `json:"receipts"`
	FirstPage bool                     `json:"firstPage"`
	LastPage  bool                     `json:"lastPage"`
}

type OtterscanAPI interface {
	GetApiLevel() uint8
	GetInternalOperations(ctx context.Context, hash common.Hash) ([]*InternalOperation, error)
	SearchTransactionsBefore(ctx context.Context, addr common.Address, blockNum uint64, pageSize uint16) (*TransactionsWithReceipts, error)
	SearchTransactionsAfter(ctx context.Context, addr common.Address, blockNum uint64, pageSize uint16) (*TransactionsWithReceipts, error)
	GetBlockDetails(ctx context.Context, number rpc.BlockNumber) (map[string]interface{}, error)
	GetBlockDetailsByHash(ctx context.Context, hash common.Hash) (map[string]interface{}, error)
	GetBlockTransactions(ctx context.Context, number rpc.BlockNumber, pageNumber uint8, pageSize uint8) (map[string]interface{}, error)
	HasCode(ctx context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (bool, error)
	TraceTransaction(ctx context.Context, hash common.Hash) ([]*TraceEntry, error)
	GetTransactionError(ctx context.Context, hash common.Hash) (hexutility.Bytes, error)
	GetTransactionBySenderAndNonce(ctx context.Context, addr common.Address, nonce uint64) (*common.Hash, error)
	GetContractCreator(ctx context.Context, addr common.Address) (*ContractCreatorData, error)
}

type OtterscanAPIImpl struct {
	*BaseAPI
	db          kv.RoDB
	maxPageSize uint64
}

func NewOtterscanAPI(base *BaseAPI, db kv.RoDB, maxPageSize uint64) *OtterscanAPIImpl {
	return &OtterscanAPIImpl{
		BaseAPI:     base,
		db:          db,
		maxPageSize: maxPageSize,
	}
}

func (api *OtterscanAPIImpl) GetApiLevel() uint8 {
	return API_LEVEL
}

// TODO: dedup from eth_txs.go#GetTransactionByHash
func (api *OtterscanAPIImpl) getTransactionByHash(ctx context.Context, tx kv.Tx, hash common.Hash) (types.Transaction, *types.Block, common.Hash, uint64, uint64, error) {
	// https://infura.io/docs/ethereum/json-rpc/eth-getTransactionByHash
	blockNum, ok, err := api.txnLookup(ctx, tx, hash)
	if err != nil {
		return nil, nil, common.Hash{}, 0, 0, err
	}
	if !ok {
		return nil, nil, common.Hash{}, 0, 0, nil
	}

	block, err := api.blockByNumberWithSenders(ctx, tx, blockNum)
	if err != nil {
		return nil, nil, common.Hash{}, 0, 0, err
	}
	if block == nil {
		return nil, nil, common.Hash{}, 0, 0, nil
	}
	blockHash := block.Hash()
	var txnIndex uint64
	var txn types.Transaction
	for i, transaction := range block.Transactions() {
		if transaction.Hash() == hash {
			txn = transaction
			txnIndex = uint64(i)
			break
		}
	}

	// Add GasPrice for the DynamicFeeTransaction
	// var baseFee *big.Int
	// if chainConfig.IsLondon(blockNum) && blockHash != (common.Hash{}) {
	// 	baseFee = block.BaseFee()
	// }

	// if no transaction was found then we return nil
	if txn == nil {
		return nil, nil, common.Hash{}, 0, 0, nil
	}
	return txn, block, blockHash, blockNum, txnIndex, nil
}

func (api *OtterscanAPIImpl) runTracer(ctx context.Context, tx kv.Tx, hash common.Hash, tracer vm.EVMLogger) (*evmtypes.ExecutionResult, error) {
	txn, block, _, _, txIndex, err := api.getTransactionByHash(ctx, tx, hash)
	if err != nil {
		return nil, err
	}
	if txn == nil {
		return nil, fmt.Errorf("transaction %#x not found", hash)
	}

	chainConfig, err := api.chainConfig(ctx, tx)
	if err != nil {
		return nil, err
	}

	engine := api.engine()

	msg, blockCtx, txCtx, ibs, _, err := transactions.ComputeTxEnv(ctx, engine, block, chainConfig, api._blockReader, tx, int(txIndex), api.historyV3(tx), false)
	if err != nil {
		return nil, err
	}

	var vmConfig vm.Config
	if tracer == nil {
		vmConfig = vm.Config{}
	} else {
		vmConfig = vm.Config{Debug: true, Tracer: tracer}
	}
	vmenv := vm.NewEVM(blockCtx, txCtx, ibs, chainConfig, vmConfig)

	result, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()).AddBlobGas(msg.BlobGas()), true, false /* gasBailout */)
	if err != nil {
		return nil, fmt.Errorf("tracing failed: %v", err)
	}

	return result, nil
}

func (api *OtterscanAPIImpl) GetInternalOperations(ctx context.Context, hash common.Hash) ([]*InternalOperation, error) {
	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	tracer := NewOperationsTracer(ctx)
	if _, err := api.runTracer(ctx, tx, hash, tracer); err != nil {
		return nil, err
	}

	return tracer.Results, nil
}

// Search transactions that touch a certain address.
//
// It searches back a certain block (excluding); the results are sorted descending.
//
// The pageSize indicates how many txs may be returned. If there are less txs than pageSize,
// they are just returned. But it may return a little more than pageSize if there are more txs
// than the necessary to fill pageSize in the last found block, i.e., let's say you want pageSize == 25,
// you already found 24 txs, the next block contains 4 matches, then this function will return 28 txs.
func (api *OtterscanAPIImpl) SearchTransactionsBefore(ctx context.Context, addr common.Address, blockNum uint64, pageSize uint16) (*TransactionsWithReceipts, error) {
	if uint64(pageSize) > api.maxPageSize {
		return nil, fmt.Errorf("max allowed page size: %v", api.maxPageSize)
	}

	dbtx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer dbtx.Rollback()

	if api.historyV3(dbtx) {
		return api.searchTransactionsBeforeV3(dbtx.(kv.TemporalTx), ctx, addr, blockNum, pageSize)
	}

	callFromCursor, err := dbtx.Cursor(kv.CallFromIndex)
	if err != nil {
		return nil, err
	}
	defer callFromCursor.Close()

	callToCursor, err := dbtx.Cursor(kv.CallToIndex)
	if err != nil {
		return nil, err
	}
	defer callToCursor.Close()

	chainConfig, err := api.chainConfig(ctx, dbtx)
	if err != nil {
		return nil, err
	}

	isFirstPage := false
	if blockNum == 0 {
		isFirstPage = true
	} else {
		// Internal search code considers blockNum [including], so adjust the value
		blockNum--
	}

	// Initialize search cursors at the first shard >= desired block number
	callFromProvider := NewCallCursorBackwardBlockProvider(callFromCursor, addr, blockNum)
	callToProvider := NewCallCursorBackwardBlockProvider(callToCursor, addr, blockNum)
	callFromToProvider := newCallFromToBlockProvider(false, callFromProvider, callToProvider)

	txs := make([]*RPCTransaction, 0, pageSize)
	receipts := make([]map[string]interface{}, 0, pageSize)

	resultCount := uint16(0)
	hasMore := true
	for {
		if resultCount >= pageSize || !hasMore {
			break
		}

		var results []*TransactionsWithReceipts
		results, hasMore, err = api.traceBlocks(ctx, addr, chainConfig, pageSize, resultCount, callFromToProvider)
		if err != nil {
			return nil, err
		}

		for _, r := range results {
			if r == nil {
				return nil, errors.New("internal error during search tracing")
			}

			for i := len(r.Txs) - 1; i >= 0; i-- {
				txs = append(txs, r.Txs[i])
			}
			for i := len(r.Receipts) - 1; i >= 0; i-- {
				receipts = append(receipts, r.Receipts[i])
			}

			resultCount += uint16(len(r.Txs))
			if resultCount >= pageSize {
				break
			}
		}
	}

	return &TransactionsWithReceipts{txs, receipts, isFirstPage, !hasMore}, nil
}

func (api *OtterscanAPIImpl) searchTransactionsBeforeV3(tx kv.TemporalTx, ctx context.Context, addr common.Address, fromBlockNum uint64, pageSize uint16) (*TransactionsWithReceipts, error) {
	chainConfig, err := api.chainConfig(ctx, tx)
	if err != nil {
		return nil, err
	}

	isFirstPage := false
	if fromBlockNum == 0 {
		isFirstPage = true
	} else {
		// Internal search code considers blockNum [including], so adjust the value
		fromBlockNum--
	}
	fromTxNum, err := rawdbv3.TxNums.Max(tx, fromBlockNum)
	if err != nil {
		return nil, err
	}
	itTo, err := tx.IndexRange(kv.TracesToIdx, addr[:], int(fromTxNum), -1, order.Desc, kv.Unlim)
	if err != nil {
		return nil, err
	}
	itFrom, err := tx.IndexRange(kv.TracesFromIdx, addr[:], int(fromTxNum), -1, order.Desc, kv.Unlim)
	if err != nil {
		return nil, err
	}
	txNums := iter.Union[uint64](itFrom, itTo, order.Desc, kv.Unlim)
	txNumsIter := MapDescendTxNum2BlockNum(tx, txNums)

	exec := txnExecutor(tx, chainConfig, api.engine(), api._blockReader, nil)
	var blockHash common.Hash
	var header *types.Header
	txs := make([]*RPCTransaction, 0, pageSize)
	receipts := make([]map[string]interface{}, 0, pageSize)
	resultCount := uint16(0)

	for txNumsIter.HasNext() {
		txNum, blockNum, txIndex, isFinalTxn, blockNumChanged, err := txNumsIter.Next()
		if err != nil {
			return nil, err
		}
		if isFinalTxn {
			continue
		}

		if blockNumChanged { // things which not changed within 1 block
			if header, err = api._blockReader.HeaderByNumber(ctx, tx, blockNum); err != nil {
				return nil, err
			}
			if header == nil {
				log.Warn("[rpc] header is nil", "blockNum", blockNum)
				continue
			}
			blockHash = header.Hash()
			exec.changeBlock(header)
		}

		//fmt.Printf("txNum=%d, blockNum=%d, txIndex=%d, maxTxNumInBlock=%d,mixTxNumInBlock=%d\n", txNum, blockNum, txIndex, maxTxNumInBlock, minTxNumInBlock)
		txn, err := api._txnReader.TxnByIdxInBlock(ctx, tx, blockNum, txIndex)
		if err != nil {
			return nil, err
		}
		if txn == nil {
			continue
		}
		rawLogs, res, err := exec.execTx(txNum, txIndex, txn)
		if err != nil {
			return nil, err
		}
		var rpcTx *RPCTransaction
		var receipt *types.Receipt
		if chainConfig.IsOptimism() {
			receipts := rawdb.ReadRawReceipts(tx, blockNum)
			if len(receipts) <= txIndex {
				return nil, fmt.Errorf("block has less receipts than expected: %d <= %d, block: %d", len(receipts), txIndex, blockNum)
			}
			receipt = receipts[txIndex]
			rpcTx = NewRPCTransaction(txn, blockHash, blockNum, uint64(txIndex), header.BaseFee, receipt)
		} else {
			rpcTx = NewRPCTransaction(txn, blockHash, blockNum, uint64(txIndex), header.BaseFee, nil)
		}
		txs = append(txs, rpcTx)
		receipt = &types.Receipt{
			Type: txn.Type(), CumulativeGasUsed: res.UsedGas,
			TransactionIndex: uint(txIndex),
			BlockNumber:      header.Number, BlockHash: blockHash, Logs: rawLogs,
		}
		mReceipt := ethutils.MarshalReceipt(receipt, txn, chainConfig, header, txn.Hash(), true)
		mReceipt["timestamp"] = header.Time
		receipts = append(receipts, mReceipt)

		resultCount++
		if resultCount >= pageSize {
			break
		}
	}
	hasMore := txNumsIter.HasNext()
	return &TransactionsWithReceipts{txs, receipts, isFirstPage, !hasMore}, nil
}

// Search transactions that touch a certain address.
//
// It searches forward a certain block (excluding); the results are sorted descending.
//
// The pageSize indicates how many txs may be returned. If there are less txs than pageSize,
// they are just returned. But it may return a little more than pageSize if there are more txs
// than the necessary to fill pageSize in the last found block, i.e., let's say you want pageSize == 25,
// you already found 24 txs, the next block contains 4 matches, then this function will return 28 txs.
func (api *OtterscanAPIImpl) SearchTransactionsAfter(ctx context.Context, addr common.Address, blockNum uint64, pageSize uint16) (*TransactionsWithReceipts, error) {
	if uint64(pageSize) > api.maxPageSize {
		return nil, fmt.Errorf("max allowed page size: %v", api.maxPageSize)
	}

	dbtx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer dbtx.Rollback()

	callFromCursor, err := dbtx.Cursor(kv.CallFromIndex)
	if err != nil {
		return nil, err
	}
	defer callFromCursor.Close()

	callToCursor, err := dbtx.Cursor(kv.CallToIndex)
	if err != nil {
		return nil, err
	}
	defer callToCursor.Close()

	chainConfig, err := api.chainConfig(ctx, dbtx)
	if err != nil {
		return nil, err
	}

	isLastPage := false
	if blockNum == 0 {
		isLastPage = true
	} else {
		// Internal search code considers blockNum [including], so adjust the value
		blockNum++
	}

	// Initialize search cursors at the first shard >= desired block number
	callFromProvider := NewCallCursorForwardBlockProvider(callFromCursor, addr, blockNum)
	callToProvider := NewCallCursorForwardBlockProvider(callToCursor, addr, blockNum)
	callFromToProvider := newCallFromToBlockProvider(true, callFromProvider, callToProvider)

	txs := make([]*RPCTransaction, 0, pageSize)
	receipts := make([]map[string]interface{}, 0, pageSize)

	resultCount := uint16(0)
	hasMore := true
	for {
		if resultCount >= pageSize || !hasMore {
			break
		}

		var results []*TransactionsWithReceipts
		results, hasMore, err = api.traceBlocks(ctx, addr, chainConfig, pageSize, resultCount, callFromToProvider)
		if err != nil {
			return nil, err
		}

		for _, r := range results {
			if r == nil {
				return nil, errors.New("internal error during search tracing")
			}

			txs = append(txs, r.Txs...)
			receipts = append(receipts, r.Receipts...)

			resultCount += uint16(len(r.Txs))
			if resultCount >= pageSize {
				break
			}
		}
	}

	// Reverse results
	lentxs := len(txs)
	for i := 0; i < lentxs/2; i++ {
		txs[i], txs[lentxs-1-i] = txs[lentxs-1-i], txs[i]
		receipts[i], receipts[lentxs-1-i] = receipts[lentxs-1-i], receipts[i]
	}
	return &TransactionsWithReceipts{txs, receipts, !hasMore, isLastPage}, nil
}

func (api *OtterscanAPIImpl) traceBlocks(ctx context.Context, addr common.Address, chainConfig *chain.Config, pageSize, resultCount uint16, callFromToProvider BlockProvider) ([]*TransactionsWithReceipts, bool, error) {
	// Estimate the common case of user address having at most 1 interaction/block and
	// trace N := remaining page matches as number of blocks to trace concurrently.
	// TODO: this is not optimimal for big contract addresses; implement some better heuristics.
	estBlocksToTrace := pageSize - resultCount
	results := make([]*TransactionsWithReceipts, estBlocksToTrace)
	totalBlocksTraced := 0
	hasMore := true

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(1024) // we don't want limit much here, but protecting from infinity attack
	for i := 0; i < int(estBlocksToTrace); i++ {
		i := i // we will pass it to goroutine

		var nextBlock uint64
		var err error
		nextBlock, hasMore, err = callFromToProvider()
		if err != nil {
			return nil, false, err
		}
		// TODO: nextBlock == 0 seems redundant with hasMore == false
		if !hasMore && nextBlock == 0 {
			break
		}

		totalBlocksTraced++

		eg.Go(func() error {
			// don't return error from searchTraceBlock - to avoid 1 block fail impact to other blocks
			// if return error - `errgroup` will interrupt all other goroutines
			// but passing `ctx` - then user still can cancel request
			api.searchTraceBlock(ctx, addr, chainConfig, i, nextBlock, results)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, false, err
	}

	return results[:totalBlocksTraced], hasMore, nil
}

func delegateGetBlockByNumber(tx kv.Tx, b *types.Block, number rpc.BlockNumber, inclTx bool) (map[string]interface{}, error) {
	td, err := rawdb.ReadTd(tx, b.Hash(), b.NumberU64())
	if err != nil {
		return nil, err
	}
	additionalFields := make(map[string]interface{})
	receipts := rawdb.ReadRawReceipts(tx, uint64(number.Int64()))
	response, err := ethapi.RPCMarshalBlock(b, inclTx, inclTx, additionalFields, receipts)
	if !inclTx {
		delete(response, "transactions") // workaround for https://github.com/erigontech/erigon/issues/4989#issuecomment-1218415666
	}
	response["totalDifficulty"] = (*hexutil2.Big)(td)
	response["transactionCount"] = b.Transactions().Len()

	if err == nil && number == rpc.PendingBlockNumber {
		// Pending blocks need to nil out a few fields
		for _, field := range []string{"hash", "nonce", "miner"} {
			response[field] = nil
		}
	}

	// Explicitly drop unwanted fields
	response["logsBloom"] = nil
	return response, err
}

// TODO: temporary workaround due to API breakage from watch_the_burn
type internalIssuance struct {
	BlockReward string `json:"blockReward,omitempty"`
	UncleReward string `json:"uncleReward,omitempty"`
	Issuance    string `json:"issuance,omitempty"`
}

func delegateIssuance(tx kv.Tx, block *types.Block, chainConfig *chain.Config, engine consensus.EngineReader) (internalIssuance, error) {
	// TODO: aura seems to be already broken in the original version of this RPC method
	rewards, err := engine.CalculateRewards(chainConfig, block.HeaderNoCopy(), block.Uncles(), func(contract common.Address, data []byte) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		return internalIssuance{}, err
	}

	blockReward := uint256.NewInt(0)
	uncleReward := uint256.NewInt(0)
	for _, r := range rewards {
		if r.Kind == consensus.RewardAuthor {
			blockReward.Add(blockReward, &r.Amount)
		}
		if r.Kind == consensus.RewardUncle {
			uncleReward.Add(uncleReward, &r.Amount)
		}
	}

	var ret internalIssuance
	ret.BlockReward = hexutil2.EncodeBig(blockReward.ToBig())
	ret.UncleReward = hexutil2.EncodeBig(uncleReward.ToBig())

	blockReward.Add(blockReward, uncleReward)
	ret.Issuance = hexutil2.EncodeBig(blockReward.ToBig())
	return ret, nil
}

func delegateBlockFees(ctx context.Context, tx kv.Tx, block *types.Block, senders []common.Address, chainConfig *chain.Config, receipts types.Receipts) (*big.Int, error) {
	fee := big.NewInt(0)
	gasUsed := big.NewInt(0)

	totalFees := big.NewInt(0)
	for _, receipt := range receipts {
		txn := block.Transactions()[receipt.TransactionIndex]
		effectiveGasPrice := uint64(0)
		if !chainConfig.IsLondon(block.NumberU64()) {
			effectiveGasPrice = txn.GetPrice().Uint64()
		} else {
			baseFee, _ := uint256.FromBig(block.BaseFee())
			gasPrice := new(big.Int).Add(block.BaseFee(), txn.GetEffectiveGasTip(baseFee).ToBig())
			effectiveGasPrice = gasPrice.Uint64()
		}

		fee.SetUint64(effectiveGasPrice)
		gasUsed.SetUint64(receipt.GasUsed)
		fee.Mul(fee, gasUsed)

		totalFees.Add(totalFees, fee)
	}

	return totalFees, nil
}

func (api *OtterscanAPIImpl) getBlockWithSenders(ctx context.Context, number rpc.BlockNumber, tx kv.Tx) (*types.Block, []common.Address, error) {
	if number == rpc.PendingBlockNumber {
		return api.pendingBlock(), nil, nil
	}

	n, hash, _, err := rpchelper.GetBlockNumber(rpc.BlockNumberOrHashWithNumber(number), tx, api.filters)
	if err != nil {
		return nil, nil, err
	}

	block, err := api.blockWithSenders(ctx, tx, hash, n)
	if err != nil {
		return nil, nil, err
	}
	if block == nil {
		return nil, nil, nil
	}
	return block, block.Body().SendersFromTxs(), nil
}

func (api *OtterscanAPIImpl) GetBlockTransactions(ctx context.Context, number rpc.BlockNumber, pageNumber uint8, pageSize uint8) (map[string]interface{}, error) {
	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b, senders, err := api.getBlockWithSenders(ctx, number, tx)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, nil
	}

	chainConfig, err := api.chainConfig(ctx, tx)
	if err != nil {
		return nil, err
	}

	getBlockRes, err := delegateGetBlockByNumber(tx, b, number, true)
	if err != nil {
		return nil, err
	}

	// Receipts
	receipts, err := api.getReceipts(ctx, tx, b, senders)
	if err != nil {
		return nil, fmt.Errorf("getReceipts error: %v", err)
	}

	result := make([]map[string]interface{}, 0, len(receipts))
	for _, receipt := range receipts {
		txn := b.Transactions()[receipt.TransactionIndex]
		marshalledRcpt := ethutils.MarshalReceipt(receipt, txn, chainConfig, b.HeaderNoCopy(), txn.Hash(), true)
		marshalledRcpt["logs"] = nil
		marshalledRcpt["logsBloom"] = nil
		result = append(result, marshalledRcpt)
	}

	// Pruned block attrs
	prunedBlock := map[string]interface{}{}
	for _, k := range []string{"timestamp", "miner", "baseFeePerGas"} {
		prunedBlock[k] = getBlockRes[k]
	}

	// Crop tx input to 4bytes
	var txs = getBlockRes["transactions"].([]interface{})
	for _, rawTx := range txs {
		rpcTx := rawTx.(*ethapi.RPCTransaction)
		if len(rpcTx.Input) >= 4 {
			rpcTx.Input = rpcTx.Input[:4]
		}
	}

	// Crop page
	pageEnd := b.Transactions().Len() - int(pageNumber)*int(pageSize)
	pageStart := pageEnd - int(pageSize)
	if pageEnd < 0 {
		pageEnd = 0
	}
	if pageStart < 0 {
		pageStart = 0
	}

	response := map[string]interface{}{}
	getBlockRes["transactions"] = getBlockRes["transactions"].([]interface{})[pageStart:pageEnd]
	response["fullblock"] = getBlockRes
	response["receipts"] = result[pageStart:pageEnd]
	return response, nil
}
