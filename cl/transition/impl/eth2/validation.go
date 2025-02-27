package eth2

import (
	"fmt"

	"github.com/Giulio2002/bls"
	"github.com/erigontech/erigon/cl/abstract"
	"github.com/erigontech/erigon/cl/cltypes"
	"github.com/erigontech/erigon/cl/fork"
	"github.com/erigontech/erigon/cl/phase1/core/state"
)

func (I *impl) VerifyTransition(s abstract.BeaconState, currentBlock *cltypes.BeaconBlock) error {
	if !I.FullValidation {
		return nil
	}
	expectedStateRoot, err := s.HashSSZ()
	if err != nil {
		return fmt.Errorf("unable to generate state root: %v", err)
	}
	if expectedStateRoot != currentBlock.StateRoot {
		return fmt.Errorf("expected state root differs from received state root, slot %d", currentBlock.Slot)
	}
	return nil
}

func (I *impl) VerifyBlockSignature(s abstract.BeaconState, block *cltypes.SignedBeaconBlock) error {
	if !I.FullValidation {
		return nil
	}
	valid, err := verifyBlockSignature(s, block)
	if err != nil {
		return fmt.Errorf("error validating block signature: %v", err)
	}
	if !valid {
		return fmt.Errorf("block not valid")
	}
	return nil
}

func verifyBlockSignature(s abstract.BeaconState, block *cltypes.SignedBeaconBlock) (bool, error) {
	proposer, err := s.ValidatorForValidatorIndex(int(block.Block.ProposerIndex))
	if err != nil {
		return false, err
	}
	domain, err := s.GetDomain(s.BeaconConfig().DomainBeaconProposer, state.Epoch(s))
	if err != nil {
		return false, err
	}
	sigRoot, err := fork.ComputeSigningRoot(block.Block, domain)
	if err != nil {
		return false, err
	}
	pk := proposer.PublicKey()
	return bls.Verify(block.Signature[:], sigRoot[:], pk[:])
}
