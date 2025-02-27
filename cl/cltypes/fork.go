package cltypes

import (
	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon/cl/merkle_tree"
	ssz2 "github.com/erigontech/erigon/cl/ssz"
)

// Fork data, contains if we were on bellatrix/alteir/phase0 and transition epoch.
type Fork struct {
	PreviousVersion libcommon.Bytes4 `json:"previous_version"`
	CurrentVersion  libcommon.Bytes4 `json:"current_version"`
	Epoch           uint64           `json:"epoch,string"`
}

func (*Fork) Static() bool {
	return true
}

func (f *Fork) Copy() *Fork {
	return &Fork{
		PreviousVersion: f.PreviousVersion,
		CurrentVersion:  f.CurrentVersion,
		Epoch:           f.Epoch,
	}
}

func (f *Fork) EncodeSSZ(dst []byte) ([]byte, error) {
	return ssz2.MarshalSSZ(dst, f.PreviousVersion[:], f.CurrentVersion[:], f.Epoch)
}

func (f *Fork) DecodeSSZ(buf []byte, _ int) error {
	return ssz2.UnmarshalSSZ(buf, 0, f.PreviousVersion[:], f.CurrentVersion[:], &f.Epoch)

}

func (f *Fork) EncodingSizeSSZ() int {
	return 16
}

func (f *Fork) HashSSZ() ([32]byte, error) {
	return merkle_tree.HashTreeRoot(f.PreviousVersion[:], f.CurrentVersion[:], f.Epoch)
}
