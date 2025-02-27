// Copyright 2014 The go-ethereum Authors
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

package core

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/holiman/uint256"

	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/fixedgas"
	"github.com/erigontech/erigon-lib/txpool/txpoolcfg"
	types2 "github.com/erigontech/erigon-lib/types"

	cmath "github.com/erigontech/erigon/common/math"
	"github.com/erigontech/erigon/common/u256"
	"github.com/erigontech/erigon/consensus/misc"
	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/core/vm"
	"github.com/erigontech/erigon/core/vm/evmtypes"
	"github.com/erigontech/erigon/crypto"
	"github.com/erigontech/erigon/params"
)

var emptyCodeHash = crypto.Keccak256Hash(nil)

/*
The State Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==

	4a) Attempt to run transaction data
	4b) If valid, use result as code for the new state object

== end ==
5) Run Script section
6) Derive new state root
*/
type StateTransition struct {
	gp           *GasPool
	msg          Message
	gasRemaining uint64
	gasPrice     *uint256.Int
	gasFeeCap    *uint256.Int
	tip          *uint256.Int
	initialGas   uint64
	value        *uint256.Int
	data         []byte
	state        evmtypes.IntraBlockState
	evm          *vm.EVM

	//some pre-allocated intermediate variables
	sharedBuyGas        *uint256.Int
	sharedBuyGasBalance *uint256.Int
}

// Message represents a message sent to a contract.
type Message interface {
	From() libcommon.Address
	To() *libcommon.Address

	GasPrice() *uint256.Int
	FeeCap() *uint256.Int
	Tip() *uint256.Int
	Gas() uint64
	BlobGas() uint64
	MaxFeePerBlobGas() *uint256.Int
	Value() *uint256.Int

	// Mint is nil if there is no minting
	Mint() *uint256.Int
	IsSystemTx() bool
	IsDepositTx() bool
	RollupCostData() types2.RollupCostData

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
	AccessList() types2.AccessList
	BlobHashes() []libcommon.Hash
	Authorizations() []types.Authorization

	IsFree() bool
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
// TODO: convert the input to a struct
func IntrinsicGas(data []byte, accessList types2.AccessList, isContractCreation bool, isHomestead, isEIP2028, isEIP3860 bool, authorizationsLen uint64) (uint64, error) {
	// Zero and non-zero bytes are priced differently
	dataLen := uint64(len(data))
	dataNonZeroLen := uint64(0)
	for _, byt := range data {
		if byt != 0 {
			dataNonZeroLen++
		}
	}

	gas, status := txpoolcfg.CalcIntrinsicGas(dataLen, dataNonZeroLen, authorizationsLen, accessList, isContractCreation, isHomestead, isEIP2028, isEIP3860)
	if status != txpoolcfg.Success {
		return 0, ErrGasUintOverflow
	}
	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	return &StateTransition{
		gp:        gp,
		evm:       evm,
		msg:       msg,
		gasPrice:  msg.GasPrice(),
		gasFeeCap: msg.FeeCap(),
		tip:       msg.Tip(),
		value:     msg.Value(),
		data:      msg.Data(),
		state:     evm.IntraBlockState(),

		sharedBuyGas:        uint256.NewInt(0),
		sharedBuyGasBalance: uint256.NewInt(0),
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
// `refunds` is false when it is not required to apply gas refunds
// `gasBailout` is true when it is not required to fail transaction if the balance is not enough to pay gas.
// for trace_call to replicate OE/Parity behaviour
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool, refunds bool, gasBailout bool) (*evmtypes.ExecutionResult, error) {
	return NewStateTransition(evm, msg, gp).TransitionDb(refunds, gasBailout)
}

// to returns the recipient of the message.
func (st *StateTransition) to() libcommon.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return libcommon.Address{}
	}
	return *st.msg.To()
}

func (st *StateTransition) buyGas(gasBailout bool) error {
	gasVal := st.sharedBuyGas
	gasVal.SetUint64(st.msg.Gas())
	gasVal, overflow := gasVal.MulOverflow(gasVal, st.gasPrice)
	if overflow {
		return fmt.Errorf("%w: address %v", ErrInsufficientFunds, st.msg.From().Hex())
	}
	var l1Cost *uint256.Int
	if fn := st.evm.Context.L1CostFunc; fn != nil {
		l1Cost = fn(st.msg.RollupCostData(), st.evm.Context.Time)
	}
	if l1Cost != nil {
		gasVal = gasVal.Add(gasVal, l1Cost)
	}

	// compute blob fee for eip-4844 data blobs if any
	blobGasVal := new(uint256.Int)
	if st.evm.ChainRules().IsCancun {
		if st.evm.Context.ExcessBlobGas == nil {
			return fmt.Errorf("%w: Cancun is active but ExcessBlobGas is nil", ErrInternalFailure)
		}
		blobGasPrice, err := misc.GetBlobGasPrice(st.evm.ChainConfig(), *st.evm.Context.ExcessBlobGas)
		if err != nil {
			return err
		}
		blobGasVal, overflow = blobGasVal.MulOverflow(blobGasPrice, new(uint256.Int).SetUint64(st.msg.BlobGas()))
		if overflow {
			return fmt.Errorf("%w: overflow converting blob gas: %v", ErrInsufficientFunds, blobGasVal)
		}
		if err := st.gp.SubBlobGas(st.msg.BlobGas()); err != nil {
			return err
		}
	}

	if !gasBailout {
		balanceCheck := gasVal
		if st.gasFeeCap != nil {
			balanceCheck = st.sharedBuyGasBalance.SetUint64(st.msg.Gas())
			balanceCheck, overflow = balanceCheck.MulOverflow(balanceCheck, st.gasFeeCap)
			if overflow {
				return fmt.Errorf("%w: address %v", ErrInsufficientFunds, st.msg.From().Hex())
			}
			balanceCheck, overflow = balanceCheck.AddOverflow(balanceCheck, st.value)
			if overflow {
				return fmt.Errorf("%w: address %v", ErrInsufficientFunds, st.msg.From().Hex())
			}
			if st.evm.ChainRules().IsCancun {
				maxBlobFee, overflow := new(uint256.Int).MulOverflow(st.msg.MaxFeePerBlobGas(), new(uint256.Int).SetUint64(st.msg.BlobGas()))
				if overflow {
					return fmt.Errorf("%w: address %v", ErrInsufficientFunds, st.msg.From().Hex())
				}
				balanceCheck, overflow = balanceCheck.AddOverflow(balanceCheck, maxBlobFee)
				if overflow {
					return fmt.Errorf("%w: address %v", ErrInsufficientFunds, st.msg.From().Hex())
				}
			}
		}
		if l1Cost != nil {
			balanceCheck, overflow = balanceCheck.AddOverflow(balanceCheck, l1Cost) // FIXME - combine with dgval below
			if overflow {
				return fmt.Errorf("%w: address %v", ErrInsufficientFunds, st.msg.From().Hex())
			}
		}
		if have, want := st.state.GetBalance(st.msg.From()), balanceCheck; have.Cmp(want) < 0 {
			return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), have, want)
		}
		st.state.SubBalance(st.msg.From(), gasVal)
		st.state.SubBalance(st.msg.From(), blobGasVal)
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gasRemaining += st.msg.Gas()
	st.initialGas = st.msg.Gas()
	st.evm.BlobFee = blobGasVal
	return nil
}

func CheckEip1559TxGasFeeCap(from libcommon.Address, gasFeeCap, tip, baseFee *uint256.Int, isFree bool) error {
	if gasFeeCap.Lt(tip) {
		return fmt.Errorf("%w: address %v, tip: %s, gasFeeCap: %s", ErrTipAboveFeeCap,
			from.Hex(), tip, gasFeeCap)
	}
	if baseFee != nil && gasFeeCap.Lt(baseFee) && !isFree {
		return fmt.Errorf("%w: address %v, gasFeeCap: %s baseFee: %s", ErrFeeCapTooLow,
			from.Hex(), gasFeeCap, baseFee)
	}
	return nil
}

// DESCRIBED: docs/programmers_guide/guide.md#nonce
func (st *StateTransition) preCheck(gasBailout bool) error {
	if st.msg.IsDepositTx() {
		// Check clause 6: caller has enough balance to cover asset transfer for **topmost** call
		// buyGas method originally handled balance check, but deposit tx does not use it
		// Therefore explicit check required for separating consensus error and evm internal error.
		// If not check it here, it will trigger evm internal error and break consensus.

		have, want := st.state.GetBalance(st.msg.From()), st.msg.Value()
		if have.Cmp(want) < 0 {
			if !gasBailout {
				return fmt.Errorf("%w: precheck address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), have, want)
			}
		}

		// Following section copied from Optimism patchset

		// No fee fields to check, no nonce to check, and no need to check if EOA (L1 already verified it for us)
		// Gas is free, but no refunds!
		st.initialGas = st.msg.Gas()
		st.gasRemaining += st.msg.Gas() // Add gas here in order to be able to execute calls.
		// Don't touch the gas pool for system transactions
		if st.msg.IsSystemTx() {
			if st.evm.ChainRules().IsOptimismRegolith {
				return fmt.Errorf("%w: address %v", ErrSystemTxNotSupported,
					st.msg.From().Hex())
			}
			return nil
		}
		return st.gp.SubGas(st.msg.Gas()) // gas used by deposits may not be used by other txs
	}

	// Make sure this transaction's nonce is correct.
	if st.msg.CheckNonce() {
		stNonce := st.state.GetNonce(st.msg.From())
		if msgNonce := st.msg.Nonce(); stNonce < msgNonce {
			return fmt.Errorf("%w: address %v, tx: %d state: %d", ErrNonceTooHigh,
				st.msg.From().Hex(), msgNonce, stNonce)
		} else if stNonce > msgNonce {
			return fmt.Errorf("%w: address %v, tx: %d state: %d", ErrNonceTooLow,
				st.msg.From().Hex(), msgNonce, stNonce)
		} else if stNonce+1 < stNonce {
			return fmt.Errorf("%w: address %v, nonce: %d", ErrNonceMax,
				st.msg.From().Hex(), stNonce)
		}

		// Make sure the sender is an EOA (EIP-3607)
		if codeHash := st.state.GetCodeHash(st.msg.From()); codeHash != emptyCodeHash && codeHash != (libcommon.Hash{}) {
			// libcommon.Hash{} means that the sender is not in the state.
			// Historically there were transactions with 0 gas price and non-existing sender,
			// so we have to allow that.

			// eip-7702 allows tx origination from accounts having delegated designation code.
			if _, ok := st.state.GetDelegatedDesignation(st.msg.From()); !ok {
				return fmt.Errorf("%w: address %v, codehash: %s", ErrSenderNoEOA,
					st.msg.From().Hex(), codeHash)
			}
		}
	}

	// Make sure the transaction gasFeeCap is greater than the block's baseFee.
	if st.evm.ChainRules().IsLondon {
		// Skip the checks if gas fields are zero and baseFee was explicitly disabled (eth_call)
		skipCheck := st.evm.Config().NoBaseFee && st.gasFeeCap.IsZero() && st.tip.IsZero()
		if !skipCheck {
			if err := CheckEip1559TxGasFeeCap(st.msg.From(), st.gasFeeCap, st.tip, st.evm.Context.BaseFee, st.msg.IsFree()); err != nil {
				return err
			}
		}
	}
	if st.msg.BlobGas() > 0 && st.evm.ChainRules().IsCancun {
		if st.evm.Context.ExcessBlobGas == nil {
			return fmt.Errorf("%w: Cancun is active but ExcessBlobGas is nil", ErrInternalFailure)
		}
		blobGasPrice, err := misc.GetBlobGasPrice(st.evm.ChainConfig(), *st.evm.Context.ExcessBlobGas)
		if err != nil {
			return err
		}
		maxFeePerBlobGas := st.msg.MaxFeePerBlobGas()
		if !st.evm.Config().NoBaseFee && blobGasPrice.Cmp(maxFeePerBlobGas) > 0 {
			return fmt.Errorf("%w: address %v, maxFeePerBlobGas: %v blobGasPrice: %v, excessBlobGas: %v",
				ErrMaxFeePerBlobGas,
				st.msg.From().Hex(), st.msg.MaxFeePerBlobGas(), blobGasPrice, st.evm.Context.ExcessBlobGas)
		}
	}

	return st.buyGas(gasBailout)
}

func (st *StateTransition) TransitionDb(refunds bool, gasBailout bool) (*evmtypes.ExecutionResult, error) {
	if mint := st.msg.Mint(); mint != nil {
		st.state.AddBalance(st.msg.From(), mint)
	}
	var snap int
	if st.msg.IsDepositTx() {
		snap = st.state.Snapshot()
	}

	result, err := st.innerTransitionDb(refunds, gasBailout)
	// Failed deposits must still be included. Unless we cannot produce the block at all due to the gas limit.
	// On deposit failure, we rewind any state changes from after the minting, and increment the nonce.
	if err != nil && err != ErrGasLimitReached && st.msg.IsDepositTx() {
		st.state.RevertToSnapshot(snap)
		// Even though we revert the state changes, always increment the nonce for the next deposit transaction
		st.state.SetNonce(st.msg.From(), st.state.GetNonce(st.msg.From())+1)
		// Record deposits as using all their gas (matches the gas pool)
		// System Transactions are special & are not recorded as using any gas (anywhere)
		gasUsed := st.msg.Gas()
		if st.msg.IsSystemTx() && !st.evm.ChainConfig().IsOptimismRegolith(st.evm.Context.Time) {
			gasUsed = 0
		}
		result = &evmtypes.ExecutionResult{
			UsedGas:    gasUsed,
			Err:        fmt.Errorf("failed deposit: %w", err),
			ReturnData: nil,
		}
		err = nil
	}
	return result, err
}

// TransitionDb will transition the state by applying the current message and
// returning the evm execution result with following fields.
//
//   - used gas:
//     total gas used (including gas being refunded)
//   - returndata:
//     the returned data from evm
//   - concrete execution error:
//     various **EVM** error which aborts the execution,
//     e.g. ErrOutOfGas, ErrExecutionReverted
//
// However if any consensus issue encountered, return the error directly with
// nil evm execution result.
func (st *StateTransition) innerTransitionDb(refunds bool, gasBailout bool) (*evmtypes.ExecutionResult, error) {
	coinbase := st.evm.Context.Coinbase

	senderInitBalance := st.state.GetBalance(st.msg.From()).Clone()
	coinbaseInitBalance := st.state.GetBalance(coinbase).Clone()

	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 1. the nonce of the message caller is correct
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. the purchased gas is enough to cover intrinsic usage
	// 5. there is no overflow when calculating intrinsic gas
	// 6. caller has enough balance to cover asset transfer for **topmost** call

	// Check clauses 1-3 and 6, buy gas if everything is correct
	if err := st.preCheck(gasBailout); err != nil {
		return nil, err
	}
	if st.evm.Config().Debug {
		st.evm.Config().Tracer.CaptureTxStart(st.initialGas)
		defer func() {
			st.evm.Config().Tracer.CaptureTxEnd(st.gasRemaining)
		}()
	}

	msg := st.msg
	sender := vm.AccountRef(msg.From())
	contractCreation := msg.To() == nil
	rules := st.evm.ChainRules()
	vmConfig := st.evm.Config()
	isEIP3860 := vmConfig.HasEip3860(rules)
	accessTuples := slices.Clone[types2.AccessList](msg.AccessList())

	if !contractCreation {
		// Increment the nonce for the next transaction
		st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
	}

	// set code tx
	auths := msg.Authorizations()
	verifiedAuthorities := make([]libcommon.Address, 0)
	if len(auths) > 0 {
		if contractCreation {
			return nil, errors.New("contract creation not allowed with type4 txs")
		}
		var b [33]byte
		data := bytes.NewBuffer(nil)
		for i, auth := range auths {
			data.Reset()

			// 1. chainId check
			if auth.ChainID != 0 && (!rules.ChainID.IsUint64() || rules.ChainID.Uint64() != auth.ChainID) {
				log.Debug("invalid chainID, skipping", "chainId", auth.ChainID, "auth index", i)
				continue
			}

			// 2. authority recover
			authorityPtr, err := auth.RecoverSigner(data, b[:])
			if err != nil {
				log.Debug("authority recover failed, skipping", "err", err, "auth index", i)
				continue
			}
			authority := *authorityPtr

			// 3. add authority account to accesses_addresses
			verifiedAuthorities = append(verifiedAuthorities, authority)
			// authority is added to accessed_address in prepare step

			// 4. authority code should be empty or already delegated
			if codeHash := st.state.GetCodeHash(authority); codeHash != emptyCodeHash && codeHash != (libcommon.Hash{}) {
				// check for delegation
				if _, ok := st.state.GetDelegatedDesignation(authority); ok {
					// noop: has delegated designation
				} else {
					log.Debug("authority code is not empty or not delegated, skipping", "auth index", i)
					continue
				}
			}

			// 5. nonce check
			authorityNonce := st.state.GetNonce(authority)
			if authorityNonce != auth.Nonce {
				log.Debug("invalid nonce, skipping", "auth index", i)
				continue
			}

			// 6. Add PER_EMPTY_ACCOUNT_COST - PER_AUTH_BASE_COST gas to the global refund counter if authority exists in the trie.
			if st.state.Exist(authority) {
				st.state.AddRefund(fixedgas.PerEmptyAccountCost - fixedgas.PerAuthBaseCost)
			}

			// 7. set authority code
			if auth.Address == (libcommon.Address{}) {
				st.state.SetCode(authority, nil)
			} else {
				st.state.SetCode(authority, types.AddressToDelegation(auth.Address))
			}

			// 8. increase the nonce of authority
			st.state.SetNonce(authority, authorityNonce+1)
		}
	}

	// Check clauses 4-5, subtract intrinsic gas if everything is correct
	gas, err := IntrinsicGas(st.data, accessTuples, contractCreation, rules.IsHomestead, rules.IsIstanbul, isEIP3860, uint64(len(auths)))
	if err != nil {
		return nil, err
	}
	if st.gasRemaining < gas {
		return nil, fmt.Errorf("%w: have %d, want %d", ErrIntrinsicGas, st.gasRemaining, gas)
	}
	st.gasRemaining -= gas

	var bailout bool
	// Gas bailout (for trace_call) should only be applied if there is not sufficient balance to perform value transfer
	if gasBailout {
		if !msg.Value().IsZero() && !st.evm.Context.CanTransfer(st.state, msg.From(), msg.Value()) {
			bailout = true
		}
	}

	// Check whether the init code size has been exceeded.
	if isEIP3860 && contractCreation && len(st.data) > params.MaxInitCodeSize {
		return nil, fmt.Errorf("%w: code size %v limit %v", ErrMaxInitCodeSizeExceeded, len(st.data), params.MaxInitCodeSize)
	}

	// Execute the preparatory steps for state transition which includes:
	// - prepare accessList(post-berlin; eip-7702)
	// - reset transient storage(eip 1153)
	st.state.Prepare(rules, msg.From(), coinbase, msg.To(), vm.ActivePrecompiles(rules), accessTuples, verifiedAuthorities)

	var (
		ret   []byte
		vmerr error // vm errors do not effect consensus and are therefore not assigned to err
	)
	if contractCreation {
		// The reason why we don't increment nonce here is that we need the original
		// nonce to calculate the address of the contract that is being created
		// It does get incremented inside the `Create` call, after the computation
		// of the contract's address, but before the execution of the code.
		ret, _, st.gasRemaining, vmerr = st.evm.Create(sender, st.data, st.gasRemaining, st.value, bailout)
	} else {
		ret, st.gasRemaining, vmerr = st.evm.Call(sender, st.to(), st.data, st.gasRemaining, st.value, bailout)
	}
	// if deposit: skip refunds, skip tipping coinbase
	// Regolith changes this behaviour to report the actual gasUsed instead of always reporting all gas used.
	if st.msg.IsDepositTx() && !rules.IsOptimismRegolith {
		// Record deposits as using all their gas (matches the gas pool)
		// System Transactions are special & are not recorded as using any gas (anywhere)
		gasUsed := st.msg.Gas()
		if st.msg.IsSystemTx() {
			gasUsed = 0
		}
		return &evmtypes.ExecutionResult{
			UsedGas:    gasUsed,
			Err:        vmerr,
			ReturnData: ret,
		}, nil
	}
	// Note for deposit tx there is no ETH refunded for unused gas, but that's taken care of by the fact that gasPrice
	// is always 0 for deposit tx. So calling refundGas will ensure the gasUsed accounting is correct without actually
	// changing the sender's balance
	if refunds {
		if rules.IsLondon {
			// After EIP-3529: refunds are capped to gasUsed / 5
			st.refundGas(params.RefundQuotientEIP3529)
		} else {
			// Before EIP-3529: refunds were capped to gasUsed / 2
			st.refundGas(params.RefundQuotient)
		}
	}
	if st.msg.IsDepositTx() && rules.IsOptimismRegolith {
		// Skip coinbase payments for deposit tx in Regolith
		return &evmtypes.ExecutionResult{
			UsedGas:    st.gasUsed(),
			Err:        vmerr,
			ReturnData: ret,
		}, nil
	}
	effectiveTip := st.gasPrice
	if rules.IsLondon {
		if st.gasFeeCap.Gt(st.evm.Context.BaseFee) {
			effectiveTip = cmath.Min256(st.tip, new(uint256.Int).Sub(st.gasFeeCap, st.evm.Context.BaseFee))
		} else {
			effectiveTip = u256.Num0
		}
	}
	amount := new(uint256.Int).SetUint64(st.gasUsed())
	amount.Mul(amount, effectiveTip) // gasUsed * effectiveTip = how much goes to the block producer (miner, validator)
	st.state.AddBalance(coinbase, amount)
	if !msg.IsFree() && rules.IsLondon {
		burntContractAddress := st.evm.ChainConfig().GetBurntContract(st.evm.Context.BlockNumber)
		if burntContractAddress != nil {
			burnAmount := new(uint256.Int).Mul(new(uint256.Int).SetUint64(st.gasUsed()), st.evm.Context.BaseFee)
			st.state.AddBalance(*burntContractAddress, burnAmount)
			if rules.IsAura && rules.IsPrague {
				// https://github.com/gnosischain/specs/blob/master/network-upgrades/pectra.md#eip-4844-pectra
				st.state.AddBalance(*burntContractAddress, st.evm.BlobFee)
			}
		}
	}

	result := &evmtypes.ExecutionResult{
		UsedGas:             st.gasUsed(),
		Err:                 vmerr,
		Reverted:            vmerr == vm.ErrExecutionReverted,
		ReturnData:          ret,
		SenderInitBalance:   senderInitBalance,
		CoinbaseInitBalance: coinbaseInitBalance,
		FeeTipped:           amount,
	}

	if st.evm.Context.PostApplyMessage != nil {
		st.evm.Context.PostApplyMessage(st.state, msg.From(), coinbase, result)
	}

	// Check that we are post bedrock to be able to create pseudo pre-bedrock blocks (these are pre-bedrock, but don't follow l2 geth rules)
	if rules.IsOptimismBedrock {
		st.state.AddBalance(params.OptimismBaseFeeRecipient, new(uint256.Int).Mul(uint256.NewInt(st.gasUsed()), st.evm.Context.BaseFee))
		if st.evm.Context.L1CostFunc == nil { // Erigon EVM context is used in many unexpected/hacky ways, let's panic if it's misconfigured
			panic("missing L1 cost func in block context, please configure l1 cost when using optimism config to run EVM")
		}
		if cost := st.evm.Context.L1CostFunc(st.msg.RollupCostData(), st.evm.Context.Time); cost != nil {
			st.state.AddBalance(params.OptimismL1FeeRecipient, cost)
		}
	}

	return result, nil
}

func (st *StateTransition) refundGas(refundQuotient uint64) {
	// Apply refund counter, capped to half of the used gas.
	refund := st.gasUsed() / refundQuotient
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gasRemaining += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(uint256.Int).Mul(new(uint256.Int).SetUint64(st.gasRemaining), st.gasPrice)
	st.state.AddBalance(st.msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gasRemaining)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gasRemaining
}
