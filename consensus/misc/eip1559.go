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
	"encoding/binary"
	"errors"
	"fmt"
	gomath "math"
	"math/big"

	"github.com/erigontech/erigon-lib/chain"
	"github.com/erigontech/erigon-lib/common"
	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon/polygon/bor/borcfg"

	"github.com/erigontech/erigon/common/math"
	"github.com/erigontech/erigon/core/rawdb"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/params"
)

// VerifyEip1559Header verifies some header attributes which were changed in EIP-1559,
// - gas limit check
// - basefee check
func VerifyEip1559Header(config *chain.Config, parent, header *types.Header, skipGasLimit bool) error {
	if !skipGasLimit {
		// Verify that the gas limit remains within allowed bounds
		parentGasLimit := parent.GasLimit
		if !config.IsLondon(parent.Number.Uint64()) {
			parentGasLimit = parent.GasLimit * config.ElasticityMultiplier(params.ElasticityMultiplier)
		}

		if config.Optimism == nil { // gasLimit can adjust instantly in optimism
			if err := VerifyGaslimit(parentGasLimit, header.GasLimit); err != nil {
				return err
			}
		}
	}
	// Verify the header is not malformed
	if header.BaseFee == nil {
		return fmt.Errorf("header is missing baseFee")
	}
	// Verify the baseFee is correct based on the parent header.
	expectedBaseFee := CalcBaseFee(config, parent, header.Time)
	if header.BaseFee.Cmp(expectedBaseFee) != 0 {
		return fmt.Errorf("invalid baseFee: have %s, want %s, parentBaseFee %s, parentGasUsed %d",
			header.BaseFee, expectedBaseFee, parent.BaseFee, parent.GasUsed)
	}
	return nil
}

var Eip1559FeeCalculator eip1559Calculator

type eip1559Calculator struct{}

func (f eip1559Calculator) CurrentFees(chainConfig *chain.Config, db kv.Getter) (baseFee, blobFee, minBlobGasPrice, blockGasLimit uint64, err error) {
	hash := rawdb.ReadHeadHeaderHash(db)

	if hash == (libcommon.Hash{}) {
		return 0, 0, 0, 0, fmt.Errorf("can't get head header hash")
	}

	currentHeader, err := rawdb.ReadHeaderByHash(db, hash)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if currentHeader == nil {
		return 0, 0, 0, 0, nil
	}

	if chainConfig != nil {
		if currentHeader.BaseFee != nil {
			baseFee = CalcBaseFee(chainConfig, currentHeader, currentHeader.Time).Uint64()
		}

		if currentHeader.ExcessBlobGas != nil {
			excessBlobGas := CalcExcessBlobGas(chainConfig, currentHeader)
			b, err := GetBlobGasPrice(chainConfig, excessBlobGas)
			if err == nil {
				blobFee = b.Uint64()
			}
		}
	}

	minBlobGasPrice = chainConfig.GetMinBlobGasPrice()

	return baseFee, blobFee, minBlobGasPrice, currentHeader.GasLimit, nil
}

// DecodeHolocene1599Params extracts the Holcene 1599 parameters from the encoded form defined here:
// https://github.com/ethereum-optimism/specs/blob/main/specs/protocol/holocene/exec-engine.md#eip-1559-parameters-in-payloadattributesv3
//
// Returns 0,0 if the format is invalid, though ValidateHolocene1559Params should be used instead of this function for
// validity checking.
func DecodeHolocene1559Params(params []byte) (uint64, uint64) {
	if len(params) != 8 {
		return 0, 0
	}
	denominator := binary.BigEndian.Uint32(params[:4])
	elasticity := binary.BigEndian.Uint32(params[4:])
	return uint64(denominator), uint64(elasticity)
}

// DecodeHoloceneExtraData decodes the Holocene 1559 parameters from the encoded form defined here:
// https://github.com/ethereum-optimism/specs/blob/main/specs/protocol/holocene/exec-engine.md#eip-1559-parameters-in-block-header
//
// Returns 0,0 if the format is invalid, though ValidateHoloceneExtraData should be used instead of this function for
// validity checking.
func DecodeHoloceneExtraData(extra []byte) (uint64, uint64) {
	if len(extra) != 9 {
		return 0, 0
	}
	return DecodeHolocene1559Params(extra[1:])
}

// EncodeHolocene1559Params encodes the eip-1559 parameters into 'PayloadAttributes.EIP1559Params' format. Will panic if
// either value is outside uint32 range.
func EncodeHolocene1559Params(denom, elasticity uint64) []byte {
	r := make([]byte, 8)
	if denom > gomath.MaxUint32 || elasticity > gomath.MaxUint32 {
		panic("eip-1559 parameters out of uint32 range")
	}
	binary.BigEndian.PutUint32(r[:4], uint32(denom))
	binary.BigEndian.PutUint32(r[4:], uint32(elasticity))
	return r
}

// EncodeHoloceneExtraData encodes the eip-1559 parameters into the header 'ExtraData' format. Will panic if either
// value is outside uint32 range.
func EncodeHoloceneExtraData(denom, elasticity uint64) []byte {
	r := make([]byte, 9)
	if denom > gomath.MaxUint32 || elasticity > gomath.MaxUint32 {
		panic("eip-1559 parameters out of uint32 range")
	}
	// leave version byte 0
	binary.BigEndian.PutUint32(r[1:5], uint32(denom))
	binary.BigEndian.PutUint32(r[5:], uint32(elasticity))
	return r
}

// ValidateHolocene1559Params checks if the encoded parameters are valid according to the Holocene
// upgrade.
func ValidateHolocene1559Params(params []byte) error {
	if len(params) != 8 {
		return fmt.Errorf("holocene eip-1559 params should be 8 bytes, got %d", len(params))
	}
	d, e := DecodeHolocene1559Params(params)
	if e != 0 && d == 0 {
		return errors.New("holocene params cannot have a 0 denominator unless elasticity is also 0")
	}
	return nil
}

// ValidateHoloceneExtraData checks if the header extraData is valid according to the Holocene
// upgrade.
func ValidateHoloceneExtraData(extra []byte) error {
	if len(extra) != 9 {
		return fmt.Errorf("holocene extraData should be 9 bytes, got %d", len(extra))
	}
	if extra[0] != 0 {
		return fmt.Errorf("holocene extraData should have 0 version byte, got %d", extra[0])
	}
	return ValidateHolocene1559Params(extra[1:])
}

// CalcBaseFee calculates the basefee of the header.
func CalcBaseFee(config *chain.Config, parent *types.Header, time uint64) *big.Int {
	// If the current block is pre bedrock, return 0.
	if config.IsOptimismPreBedrock(parent.Number.Uint64()) {
		return common.Big0
	}

	// If the current block is the first EIP-1559 block, return the InitialBaseFee.
	if !config.IsLondon(parent.Number.Uint64()) {
		return new(big.Int).SetUint64(params.InitialBaseFee)
	}

	elasticity := config.ElasticityMultiplier(params.ElasticityMultiplier)
	denominator := getBaseFeeChangeDenominator(config, parent.Number.Uint64(), time)
	if config.IsOptimism() && config.IsHolocene(parent.Time) {
		denominator, elasticity = DecodeHoloceneExtraData(parent.Extra)
		if denominator == 0 {
			// this shouldn't happen as the ExtraData should have been validated prior
			panic("invalid eip-1559 params in extradata")
		}
	}

	var (
		parentGasTarget          = parent.GasLimit / elasticity
		parentGasTargetBig       = new(big.Int).SetUint64(parentGasTarget)
		baseFeeChangeDenominator = new(big.Int).SetUint64(denominator)
	)
	// If the parent gasUsed is the same as the target, the baseFee remains unchanged.
	if parent.GasUsed == parentGasTarget {
		return new(big.Int).Set(parent.BaseFee)
	}
	if parent.GasUsed > parentGasTarget {
		// If the parent block used more gas than its target, the baseFee should increase.
		gasUsedDelta := new(big.Int).SetUint64(parent.GasUsed - parentGasTarget)
		x := new(big.Int).Mul(parent.BaseFee, gasUsedDelta)
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := math.BigMax(
			x.Div(y, baseFeeChangeDenominator),
			common.Big1,
		)

		return x.Add(parent.BaseFee, baseFeeDelta)
	} else {
		// Otherwise if the parent block used less gas than its target, the baseFee should decrease.
		gasUsedDelta := new(big.Int).SetUint64(parentGasTarget - parent.GasUsed)
		x := new(big.Int).Mul(parent.BaseFee, gasUsedDelta)
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := x.Div(y, baseFeeChangeDenominator)

		return math.BigMax(
			x.Sub(parent.BaseFee, baseFeeDelta),
			common.Big0,
		)
	}
}

func getBaseFeeChangeDenominator(config *chain.Config, number, time uint64) uint64 {
	// If we're running bor based chain post delhi hardfork, return the new value
	if borConfig, ok := config.Bor.(*borcfg.BorConfig); ok && borConfig.IsDelhi(number) {
		return params.BaseFeeChangeDenominatorPostDelhi
	}

	// Return the original once for other chains and pre-fork cases
	return config.BaseFeeChangeDenominator(params.BaseFeeChangeDenominator, time)
}
