// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package misc

import (
	"math/big"
	"testing"

	"github.com/erigontech/erigon-lib/chain"
	"github.com/erigontech/erigon-lib/common"

	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/params"
)

// copyConfig does a _shallow_ copy of a given config. Safe to set new values, but
// do not use e.g. SetInt() on the numbers. For testing only
func copyConfig(original *chain.Config) *chain.Config {
	copy := *original
	return &copy
}

func config() *chain.Config {
	config := copyConfig(params.TestChainConfig)
	config.LondonBlock = big.NewInt(5)
	return config
}

func opConfig() *chain.Config {
	config := copyConfig(params.TestChainConfig)
	config.BedrockBlock = big.NewInt(0)
	config.LondonBlock = big.NewInt(5)
	ct := big.NewInt(10)
	eip1559DenominatorCanyon := uint64(250)
	config.CanyonTime = ct
	ht := big.NewInt(12)
	config.HoloceneTime = ht
	config.Optimism = &chain.OptimismConfig{
		EIP1559Elasticity:        6,
		EIP1559Denominator:       50,
		EIP1559DenominatorCanyon: eip1559DenominatorCanyon,
	}
	return config
}

// TestBlockGasLimits tests the gasLimit checks for blocks both across
// the EIP-1559 boundary and post-1559 blocks
func TestBlockGasLimits(t *testing.T) {
	initial := new(big.Int).SetUint64(params.InitialBaseFee)

	for i, tc := range []struct {
		pGasLimit uint64
		pNum      int64
		gasLimit  uint64
		ok        bool
	}{
		// Transitions from non-london to london
		{10000000, 4, 20000000, true},  // No change
		{10000000, 4, 20019530, true},  // Upper limit
		{10000000, 4, 20019531, false}, // Upper +1
		{10000000, 4, 19980470, true},  // Lower limit
		{10000000, 4, 19980469, false}, // Lower limit -1
		// London to London
		{20000000, 5, 20000000, true},
		{20000000, 5, 20019530, true},  // Upper limit
		{20000000, 5, 20019531, false}, // Upper limit +1
		{20000000, 5, 19980470, true},  // Lower limit
		{20000000, 5, 19980469, false}, // Lower limit -1
		{40000000, 5, 40039061, true},  // Upper limit
		{40000000, 5, 40039062, false}, // Upper limit +1
		{40000000, 5, 39960939, true},  // lower limit
		{40000000, 5, 39960938, false}, // Lower limit -1
	} {
		parent := &types.Header{
			GasUsed:  tc.pGasLimit / 2,
			GasLimit: tc.pGasLimit,
			BaseFee:  initial,
			Number:   big.NewInt(tc.pNum),
		}
		header := &types.Header{
			GasUsed:  tc.gasLimit / 2,
			GasLimit: tc.gasLimit,
			BaseFee:  initial,
			Number:   big.NewInt(tc.pNum + 1),
		}
		err := VerifyEip1559Header(config(), parent, header, false /*skipGasLimit*/)
		if tc.ok && err != nil {
			t.Errorf("test %d: Expected valid header: %s", i, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("test %d: Expected invalid header", i)
		}
	}
}

// TestCalcBaseFee assumes all blocks are 1559-blocks
func TestCalcBaseFee(t *testing.T) {
	tests := []struct {
		parentBaseFee   int64
		parentGasLimit  uint64
		parentGasUsed   uint64
		expectedBaseFee int64
	}{
		{params.InitialBaseFee, 20000000, 10000000, params.InitialBaseFee}, // usage == target
		{params.InitialBaseFee, 20000000, 9000000, 987500000},              // usage below target
		{params.InitialBaseFee, 20000000, 11000000, 1012500000},            // usage above target
	}
	for i, test := range tests {
		parent := &types.Header{
			Number:   common.Big32,
			GasLimit: test.parentGasLimit,
			GasUsed:  test.parentGasUsed,
			BaseFee:  big.NewInt(test.parentBaseFee),
		}
		if have, want := CalcBaseFee(config(), parent, 0), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
	}
}

// TestCalcBaseFeeOptimism assumes all blocks are 1559-blocks but tests the Canyon activation
func TestCalcBaseFeeOptimism(t *testing.T) {
	tests := []struct {
		parentBaseFee   int64
		parentGasLimit  uint64
		parentGasUsed   uint64
		expectedBaseFee int64
		postCanyon      bool
	}{
		{params.InitialBaseFee, 30_000_000, 5_000_000, params.InitialBaseFee, false}, // usage == target
		{params.InitialBaseFee, 30_000_000, 4_000_000, 996000000, false},             // usage below target
		{params.InitialBaseFee, 30_000_000, 10_000_000, 1020000000, false},           // usage above target
		{params.InitialBaseFee, 30_000_000, 5_000_000, params.InitialBaseFee, true},  // usage == target
		{params.InitialBaseFee, 30_000_000, 4_000_000, 999200000, true},              // usage below target
		{params.InitialBaseFee, 30_000_000, 10_000_000, 1004000000, true},            // usage above target
	}
	for i, test := range tests {
		parent := &types.Header{
			Number:   common.Big32,
			GasLimit: test.parentGasLimit,
			GasUsed:  test.parentGasUsed,
			BaseFee:  big.NewInt(test.parentBaseFee),
			Time:     6,
		}
		if test.postCanyon {
			parent.Time = 8
		}
		if have, want := CalcBaseFee(opConfig(), parent, parent.Time+2), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
		if test.postCanyon {
			// make sure Holocene activation doesn't change the outcome; since these tests have empty eip1559 params,
			// they should be handled using the Canyon config.
			parent.Time = 10
			if have, want := CalcBaseFee(opConfig(), parent, parent.Time+2), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
				t.Errorf("test %d: have %d  want %d, ", i, have, want)
			}
		}
	}
}

// TestCalcBaseFeeHolocene assumes all blocks are Optimism blocks post-Holocene upgrade
func TestCalcBaseFeeOptimismHolocene(t *testing.T) {
	parentBaseFee := int64(10_000_000)
	parentGasLimit := uint64(30_000_000)

	tests := []struct {
		parentGasUsed     uint64
		expectedBaseFee   int64
		denom, elasticity uint64
	}{
		{parentGasLimit / 2, parentBaseFee, 10, 2},  // target
		{10_000_000, 9_666_667, 10, 2},              // below
		{20_000_000, 10_333_333, 10, 2},             // above
		{parentGasLimit / 10, parentBaseFee, 2, 10}, // target
		{1_000_000, 6_666_667, 2, 10},               // below
		{30_000_000, 55_000_000, 2, 10},             // above
	}
	for i, test := range tests {
		parent := &types.Header{
			Number:   common.Big32,
			GasLimit: parentGasLimit,
			GasUsed:  test.parentGasUsed,
			BaseFee:  big.NewInt(parentBaseFee),
			Time:     12,
			Extra:    EncodeHoloceneExtraData(test.denom, test.elasticity),
		}
		if have, want := CalcBaseFee(opConfig(), parent, parent.Time+2), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
	}
}
