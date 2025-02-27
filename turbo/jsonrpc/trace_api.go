package jsonrpc

import (
	"context"
	"encoding/json"

	jsoniter "github.com/json-iterator/go"

	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon/cmd/rpcdaemon/cli/httpcfg"
	"github.com/erigontech/erigon/eth/tracers"
	"github.com/erigontech/erigon/rpc"
)

// TraceAPI RPC interface into tracing API
type TraceAPI interface {
	// Ad-hoc (see ./trace_adhoc.go)

	ReplayBlockTransactions(ctx context.Context, blockNr rpc.BlockNumberOrHash, traceTypes []string, gasBailOut *bool, traceConfig *tracers.TraceConfig) ([]*TraceCallResult, error)
	ReplayTransaction(ctx context.Context, txHash libcommon.Hash, traceTypes []string, gasBailOut *bool, traceConfig *tracers.TraceConfig) (*TraceCallResult, error)
	Call(ctx context.Context, call TraceCallParam, types []string, blockNr *rpc.BlockNumberOrHash, traceConfig *tracers.TraceConfig) (*TraceCallResult, error)
	CallMany(ctx context.Context, calls json.RawMessage, blockNr *rpc.BlockNumberOrHash, traceConfig *tracers.TraceConfig) ([]*TraceCallResult, error)
	RawTransaction(ctx context.Context, txHash libcommon.Hash, traceTypes []string) ([]interface{}, error)

	// Filtering (see ./trace_filtering.go)

	Transaction(ctx context.Context, txHash libcommon.Hash, gasBailOut *bool, traceConfig *tracers.TraceConfig) (ParityTraces, error)
	Get(ctx context.Context, txHash libcommon.Hash, txIndicies []hexutil.Uint64, gasBailOut *bool, traceConfig *tracers.TraceConfig) (*ParityTrace, error)
	Block(ctx context.Context, blockNr rpc.BlockNumber, gasBailOut *bool, traceConfig *tracers.TraceConfig) (ParityTraces, error)
	Filter(ctx context.Context, req TraceFilterRequest, gasBailOut *bool, traceConfig *tracers.TraceConfig, stream *jsoniter.Stream) error
}

// TraceAPIImpl is implementation of the TraceAPI interface based on remote Db access
type TraceAPIImpl struct {
	*BaseAPI
	kv            kv.RoDB
	maxTraces     uint64
	gasCap        uint64
	compatibility bool // Bug for bug compatiblity with OpenEthereum
}

// NewTraceAPI returns NewTraceAPI instance
func NewTraceAPI(base *BaseAPI, kv kv.RoDB, cfg *httpcfg.HttpCfg) *TraceAPIImpl {
	return &TraceAPIImpl{
		BaseAPI:       base,
		kv:            kv,
		maxTraces:     cfg.MaxTraces,
		gasCap:        cfg.Gascap,
		compatibility: cfg.TraceCompatibility,
	}
}
