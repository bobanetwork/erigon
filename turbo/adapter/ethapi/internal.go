package ethapi

// This file stores proxy-objects for `internal` package
import (
	libcommon "github.com/erigontech/erigon-lib/common"

	"github.com/erigontech/erigon/core/types"
)

// nolint
func RPCMarshalBlock(b *types.Block, inclTx bool, fullTx bool, additional map[string]interface{}, receipts types.Receipts) (map[string]interface{}, error) {
	fields, err := RPCMarshalBlockDeprecated(b, inclTx, fullTx, receipts)
	if err != nil {
		return nil, err
	}

	for k, v := range additional {
		fields[k] = v
	}

	return fields, err
}

// nolint
func RPCMarshalBlockEx(b *types.Block, inclTx bool, fullTx bool, borTx types.Transaction, borTxHash libcommon.Hash, additional map[string]interface{}, receipts types.Receipts) (map[string]interface{}, error) {
	fields, err := RPCMarshalBlockExDeprecated(b, inclTx, fullTx, borTx, borTxHash, receipts)
	if err != nil {
		return nil, err
	}

	for k, v := range additional {
		fields[k] = v
	}

	return fields, err
}
