// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/hexutil"
	"github.com/ledgerwatch/erigon-lib/common/hexutility"
)

var _ = (*receiptMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (r Receipt) MarshalJSON() ([]byte, error) {
	type Receipt struct {
		Type              hexutil.Uint64   `json:"type,omitempty"`
		PostState         hexutility.Bytes `json:"root" codec:"1"`
		Status            hexutil.Uint64   `json:"status" codec:"2"`
		CumulativeGasUsed hexutil.Uint64   `json:"cumulativeGasUsed" gencodec:"required" codec:"3"`
		Bloom             Bloom            `json:"logsBloom"         gencodec:"required" codec:"-"`
		Logs              Logs             `json:"logs"              gencodec:"required" codec:"-"`
		TxHash            common.Hash      `json:"transactionHash" gencodec:"required" codec:"-"`
		ContractAddress   common.Address   `json:"contractAddress" codec:"-"`
		GasUsed           hexutil.Uint64   `json:"gasUsed" gencodec:"required" codec:"-"`
		DepositNonce      *uint64          `json:"depositNonce,omitempty"`
		BlockHash         common.Hash      `json:"blockHash,omitempty" codec:"-"`
		BlockNumber       *hexutil.Big     `json:"blockNumber,omitempty" codec:"-"`
		TransactionIndex  hexutil.Uint     `json:"transactionIndex" codec:"-"`
		L1GasPrice        *hexutil.Big     `json:"l1GasPrice,omitempty"`
		L1GasUsed         *hexutil.Big     `json:"l1GasUsed,omitempty"`
		L1Fee             *hexutil.Big     `json:"l1Fee,omitempty"`
		FeeScalar         *big.Float       `json:"l1FeeScalar,omitempty"`
	}
	var enc Receipt
	enc.Type = hexutil.Uint64(r.Type)
	enc.PostState = r.PostState
	enc.Status = hexutil.Uint64(r.Status)
	enc.CumulativeGasUsed = hexutil.Uint64(r.CumulativeGasUsed)
	enc.Bloom = r.Bloom
	enc.Logs = r.Logs
	enc.TxHash = r.TxHash
	enc.ContractAddress = r.ContractAddress
	enc.GasUsed = hexutil.Uint64(r.GasUsed)
	enc.DepositNonce = r.DepositNonce
	enc.BlockHash = r.BlockHash
	enc.BlockNumber = (*hexutil.Big)(r.BlockNumber)
	enc.TransactionIndex = hexutil.Uint(r.TransactionIndex)
	enc.L1GasPrice = (*hexutil.Big)(r.L1GasPrice)
	enc.L1GasUsed = (*hexutil.Big)(r.L1GasUsed)
	enc.L1Fee = (*hexutil.Big)(r.L1Fee)
	enc.FeeScalar = r.FeeScalar
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (r *Receipt) UnmarshalJSON(input []byte) error {
	type Receipt struct {
		Type              *hexutil.Uint64   `json:"type,omitempty"`
		PostState         *hexutility.Bytes `json:"root" codec:"1"`
		Status            *hexutil.Uint64   `json:"status" codec:"2"`
		CumulativeGasUsed *hexutil.Uint64   `json:"cumulativeGasUsed" gencodec:"required" codec:"3"`
		Bloom             *Bloom            `json:"logsBloom"         gencodec:"required" codec:"-"`
		Logs              *Logs             `json:"logs"              gencodec:"required" codec:"-"`
		TxHash            *common.Hash      `json:"transactionHash" gencodec:"required" codec:"-"`
		ContractAddress   *common.Address   `json:"contractAddress" codec:"-"`
		GasUsed           *hexutil.Uint64   `json:"gasUsed" gencodec:"required" codec:"-"`
		DepositNonce      *uint64           `json:"depositNonce,omitempty"`
		BlockHash         *common.Hash      `json:"blockHash,omitempty" codec:"-"`
		BlockNumber       *hexutil.Big      `json:"blockNumber,omitempty" codec:"-"`
		TransactionIndex  *hexutil.Uint     `json:"transactionIndex" codec:"-"`
		L1GasPrice        *hexutil.Big      `json:"l1GasPrice,omitempty"`
		L1GasUsed         *hexutil.Big      `json:"l1GasUsed,omitempty"`
		L1Fee             *hexutil.Big      `json:"l1Fee,omitempty"`
		FeeScalar         *big.Float        `json:"l1FeeScalar,omitempty"`
	}
	var dec Receipt
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Type != nil {
		r.Type = uint8(*dec.Type)
	}
	if dec.PostState != nil {
		r.PostState = *dec.PostState
	}
	if dec.Status != nil {
		r.Status = uint64(*dec.Status)
	}
	if dec.CumulativeGasUsed == nil {
		return errors.New("missing required field 'cumulativeGasUsed' for Receipt")
	}
	r.CumulativeGasUsed = uint64(*dec.CumulativeGasUsed)
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Receipt")
	}
	r.Bloom = *dec.Bloom
	if dec.Logs == nil {
		return errors.New("missing required field 'logs' for Receipt")
	}
	r.Logs = *dec.Logs
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionHash' for Receipt")
	}
	r.TxHash = *dec.TxHash
	if dec.ContractAddress != nil {
		r.ContractAddress = *dec.ContractAddress
	}
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Receipt")
	}
	r.GasUsed = uint64(*dec.GasUsed)
	if dec.DepositNonce != nil {
		r.DepositNonce = dec.DepositNonce
	}
	if dec.BlockHash != nil {
		r.BlockHash = *dec.BlockHash
	}
	if dec.BlockNumber != nil {
		r.BlockNumber = (*big.Int)(dec.BlockNumber)
	}
	if dec.TransactionIndex != nil {
		r.TransactionIndex = uint(*dec.TransactionIndex)
	}
	if dec.L1GasPrice != nil {
		r.L1GasPrice = (*big.Int)(dec.L1GasPrice)
	}
	if dec.L1GasUsed != nil {
		r.L1GasUsed = (*big.Int)(dec.L1GasUsed)
	}
	if dec.L1Fee != nil {
		r.L1Fee = (*big.Int)(dec.L1Fee)
	}
	if dec.FeeScalar != nil {
		r.FeeScalar = dec.FeeScalar
	}
	return nil
}
