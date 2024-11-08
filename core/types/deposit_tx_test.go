package types

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/stretchr/testify/require"
)

func TestDepositTxHash(t *testing.T) {
	dtx := DepositTx{
		SourceHash: common.HexToHash("0xc9fa17cc88928d8303f4efcc0053ddbd8c5baea5ed4c1da2efd019833070c182"),
		From:       common.HexToAddress("0x976EA74026E726554dB657fA54763abd0C3a0aa9"),
		To:         ptr(common.HexToAddress("0x976EA74026E726554dB657fA54763abd0C3a0aa9")),
		Mint:       uint256.NewInt(1_000_000_000_000),
		Value:      uint256.NewInt(0),
		Gas:        1_000_000,
	}

	require.Equal(t, common.HexToHash("0x5c7753e59abb0904e0e70a28f4d24458cf20c2b5b491365411ce54a662874197"), dtx.Hash())
}

func ptr[T any](v T) *T {
	return &v
}
