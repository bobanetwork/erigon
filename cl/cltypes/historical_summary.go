package cltypes

import (
	libcommon "github.com/erigontech/erigon-lib/common"

	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon/cl/merkle_tree"
	ssz2 "github.com/erigontech/erigon/cl/ssz"
)

type HistoricalSummary struct {
	BlockSummaryRoot libcommon.Hash `json:"block_summary_root"`
	StateSummaryRoot libcommon.Hash `json:"state_summary_root"`
}

func (h *HistoricalSummary) EncodeSSZ(buf []byte) ([]byte, error) {
	return ssz2.MarshalSSZ(buf, h.BlockSummaryRoot[:], h.StateSummaryRoot[:])
}

func (h *HistoricalSummary) DecodeSSZ(buf []byte, _ int) error {
	return ssz2.UnmarshalSSZ(buf, 0, h.BlockSummaryRoot[:], h.StateSummaryRoot[:])
}

func (h *HistoricalSummary) HashSSZ() ([32]byte, error) {
	return merkle_tree.HashTreeRoot(h.BlockSummaryRoot[:], h.StateSummaryRoot[:])
}

func (*HistoricalSummary) EncodingSizeSSZ() int {
	return length.Hash * 2
}
