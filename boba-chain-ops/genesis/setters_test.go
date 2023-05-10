package genesis

import (
	"math/big"
	"testing"

	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/boba-bindings/predeploys"
	"github.com/ledgerwatch/erigon/boba-chain-ops/immutables"
	"github.com/ledgerwatch/erigon/boba-chain-ops/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/stretchr/testify/require"
)

func TestWipePredeployStorage(t *testing.T) {
	g := &types.Genesis{
		Config: &chain.Config{
			ChainID: big.NewInt(2888),
		},
		Alloc: types.GenesisAlloc{},
	}

	code := []byte{1, 2, 3}
	storeVal := common.Hash{31: 0xff}
	nonce := 100

	for _, addr := range predeploys.Predeploys {
		a := *addr
		g.Alloc[a] = types.GenesisAccount{
			Code: code,
			Storage: map[common.Hash]common.Hash{
				storeVal: storeVal,
			},
			Nonce: uint64(nonce),
		}
	}

	WipePredeployStorage(g)

	for _, addr := range predeploys.Predeploys {
		if FrozenStoragePredeploys[*addr] {
			expected := types.GenesisAccount{
				Code: code,
				Storage: map[common.Hash]common.Hash{
					storeVal: storeVal,
				},
				Nonce: uint64(nonce),
			}
			require.Equal(t, expected, g.Alloc[*addr])
			continue
		}
		expected := types.GenesisAccount{
			Code:    code,
			Storage: map[common.Hash]common.Hash{},
			Nonce:   uint64(nonce),
		}
		require.Equal(t, expected, g.Alloc[*addr])
	}
}

func TestSetImplementations(t *testing.T) {
	g := &types.Genesis{
		Config: &chain.Config{
			ChainID: big.NewInt(2888),
		},
		Alloc: types.GenesisAlloc{},
	}

	immutables := immutables.ImmutableConfig{
		"L2StandardBridge": {
			"otherBridge": common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"L2CrossDomainMessenger": {
			"otherMessenger": common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"L2ERC721Bridge": {
			"otherBridge": common.HexToAddress("0x1234567890123456789012345678901234567890"),
			"messenger":   common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"OptimismMintableERC721Factory": {
			"remoteChainId": big.NewInt(1),
			"bridge":        common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"SequencerFeeVault": {
			"recipient": common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"L1FeeVault": {
			"recipient": common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"BaseFeeVault": {
			"recipient": common.HexToAddress("0x1234567890123456789012345678901234567890"),
		},
		"BobaL2": {
			"bridge":      common.HexToAddress("0x1234567890123456789012345678901234567890"),
			"remoteToken": common.HexToAddress("0x0123456789012345678901234567890123456789"),
		},
	}
	storage := make(state.StorageConfig)
	storage["L2ToL1MessagePasser"] = state.StorageValues{
		"msgNonce": 0,
	}
	storage["L2CrossDomainMessenger"] = state.StorageValues{
		"_initialized":     1,
		"_initializing":    false,
		"xDomainMsgSender": "0x000000000000000000000000000000000000dEaD",
		"msgNonce":         0,
	}
	storage["L1Block"] = state.StorageValues{
		"number":         common.Big1,
		"timestamp":      0,
		"basefee":        common.Big1,
		"hash":           common.Hash{1},
		"sequenceNumber": 0,
		"batcherHash":    common.Hash{1},
		"l1FeeOverhead":  0,
		"l1FeeScalar":    0,
	}
	storage["LegacyERC20ETH"] = state.StorageValues{
		"_name":   "Ether",
		"_symbol": "ETH",
	}
	storage["WETH9"] = state.StorageValues{
		"name":     "Wrapped Ether",
		"symbol":   "WETH",
		"decimals": 18,
	}
	storage["GovernanceToken"] = state.StorageValues{
		"_name":   "Test Token",
		"_symbol": "TEST",
		"_owner":  common.Address{1},
	}
	storage["ProxyAdmin"] = state.StorageValues{
		"_owner": common.Address{1},
	}
	storage["ProxyAdmin"] = state.StorageValues{
		"_owner": common.Address{1},
	}
	storage["BobaL2"] = state.StorageValues{
		"_name":   "Boba L2",
		"_symbol": "BOBA",
	}
	storage["BobaTuringCredit"] = state.StorageValues{}

	SetImplementations(g, storage, immutables)

	for name, address := range predeploys.Predeploys {
		if FrozenStoragePredeploys[*address] {
			continue
		}
		if *address == predeploys.LegacyERC20ETHAddr {
			continue
		}

		_, ok := g.Alloc[*address]
		require.True(t, ok, "predeploy %s not found in genesis", name)
		require.NotEqual(t, common.Hash{}, g.Alloc[*address].Storage[ImplementationSlot])
		codeAddr, err := AddressToCodeNamespace(*address)
		if err != nil {
			t.Fatal(err)
		}
		require.NotEqual(t, common.Hash{}, g.Alloc[codeAddr].Code)
	}
}
