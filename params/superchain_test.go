package params

import (
	"math/big"
	"testing"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/stretchr/testify/require"
)

type hardforkConfig struct {
	chainID                  uint64
	ShanghaiTime             *big.Int
	CancunTime               *big.Int
	BedrockBlock             *big.Int
	RegolithTime             *big.Int
	CanyonTime               *big.Int
	EcotoneTime              *big.Int
	FjordTime                *big.Int
	GraniteTime              *big.Int
	EIP1559Elasticity        uint64
	EIP1559Denominator       uint64
	EIP1559DenominatorCanyon uint64
}

var bobaSepoliaCfg = hardforkConfig{
	chainID:                  28882,
	ShanghaiTime:             big.NewInt(1705600788),
	CancunTime:               big.NewInt(1709078400),
	BedrockBlock:             big.NewInt(511),
	RegolithTime:             big.NewInt(1705600788),
	CanyonTime:               big.NewInt(1705600788),
	EcotoneTime:              big.NewInt(1709078400),
	FjordTime:                big.NewInt(1722297600),
	GraniteTime:              big.NewInt(1726470000),
	EIP1559Elasticity:        6,
	EIP1559Denominator:       50,
	EIP1559DenominatorCanyon: 250,
}

var bobaMainnetCfg = hardforkConfig{
	chainID:                  288,
	ShanghaiTime:             big.NewInt(1713302879),
	CancunTime:               big.NewInt(1713302880),
	BedrockBlock:             big.NewInt(1149019),
	RegolithTime:             big.NewInt(1713302879),
	CanyonTime:               big.NewInt(1713302879),
	EcotoneTime:              big.NewInt(1713302880),
	FjordTime:                big.NewInt(1725951600),
	GraniteTime:              big.NewInt(1729753200),
	EIP1559Elasticity:        6,
	EIP1559Denominator:       50,
	EIP1559DenominatorCanyon: 250,
}

var bobaBnbTestnetCfg = hardforkConfig{
	chainID:                  9728,
	ShanghaiTime:             big.NewInt(1718920167),
	CancunTime:               big.NewInt(1718920168),
	BedrockBlock:             big.NewInt(675077),
	RegolithTime:             big.NewInt(1718920167),
	CanyonTime:               big.NewInt(1718920167),
	EcotoneTime:              big.NewInt(1718920168),
	FjordTime:                big.NewInt(1722297600),
	GraniteTime:              big.NewInt(1726470000),
	EIP1559Elasticity:        6,
	EIP1559Denominator:       50,
	EIP1559DenominatorCanyon: 250,
}

var bobaSepoliaDev0Cfg = hardforkConfig{
	chainID:                  288882,
	ShanghaiTime:             big.NewInt(1724692140),
	CancunTime:               big.NewInt(1724692141),
	BedrockBlock:             big.NewInt(0),
	RegolithTime:             big.NewInt(0),
	CanyonTime:               big.NewInt(1724692140),
	EcotoneTime:              big.NewInt(1724692141),
	FjordTime:                big.NewInt(1724692150),
	GraniteTime:              big.NewInt(1724914800),
	EIP1559Elasticity:        6,
	EIP1559Denominator:       50,
	EIP1559DenominatorCanyon: 250,
}

var opSepoliaCfg = hardforkConfig{
	chainID:                  11155420,
	ShanghaiTime:             big.NewInt(1699981200),
	CancunTime:               big.NewInt(1708534800),
	BedrockBlock:             big.NewInt(0),
	RegolithTime:             big.NewInt(0),
	CanyonTime:               big.NewInt(1699981200),
	EcotoneTime:              big.NewInt(1708534800),
	FjordTime:                big.NewInt(1716998400),
	GraniteTime:              big.NewInt(1723478400),
	EIP1559Elasticity:        6,
	EIP1559Denominator:       50,
	EIP1559DenominatorCanyon: 250,
}

var opMainnetCfg = hardforkConfig{
	chainID:                  10,
	ShanghaiTime:             big.NewInt(1704992401),
	CancunTime:               big.NewInt(1710374401),
	BedrockBlock:             big.NewInt(105235063),
	RegolithTime:             big.NewInt(0),
	CanyonTime:               big.NewInt(1704992401),
	EcotoneTime:              big.NewInt(1710374401),
	FjordTime:                big.NewInt(1720627201),
	GraniteTime:              big.NewInt(1726070401),
	EIP1559Elasticity:        6,
	EIP1559Denominator:       50,
	EIP1559DenominatorCanyon: 250,
}

func TestChainConfigByOpStackChainName(t *testing.T) {
	hardforkConfigsByName := map[string]hardforkConfig{
		"boba-sepolia":     bobaSepoliaCfg,
		"boba-mainnet":     bobaMainnetCfg,
		"boba-bnb-testnet": bobaBnbTestnetCfg,
		"op-sepolia":       opSepoliaCfg,
		"op-mainnet":       opMainnetCfg,
	}

	for name, expectedHarhardforkCfg := range hardforkConfigsByName {
		gotCfg := ChainConfigByOpStackChainName(name)
		require.NotNil(t, gotCfg)

		// ChainID
		require.Equal(t, expectedHarhardforkCfg.chainID, gotCfg.ChainID.Uint64())

		// Hardforks
		require.Equal(t, expectedHarhardforkCfg.ShanghaiTime, gotCfg.ShanghaiTime)
		require.Equal(t, expectedHarhardforkCfg.CancunTime, gotCfg.CancunTime)
		require.Equal(t, expectedHarhardforkCfg.BedrockBlock, gotCfg.BedrockBlock)
		require.Equal(t, expectedHarhardforkCfg.RegolithTime, gotCfg.RegolithTime)
		require.Equal(t, expectedHarhardforkCfg.CanyonTime, gotCfg.CanyonTime)
		require.Equal(t, expectedHarhardforkCfg.EcotoneTime, gotCfg.EcotoneTime)
		require.Equal(t, expectedHarhardforkCfg.FjordTime, gotCfg.FjordTime)
		require.Equal(t, expectedHarhardforkCfg.GraniteTime, gotCfg.GraniteTime)

		// EIP-1559
		require.Equal(t, expectedHarhardforkCfg.EIP1559Elasticity, gotCfg.Optimism.EIP1559Elasticity)
		require.Equal(t, expectedHarhardforkCfg.EIP1559Denominator, gotCfg.Optimism.EIP1559Denominator)
		require.Equal(t, expectedHarhardforkCfg.EIP1559DenominatorCanyon, gotCfg.Optimism.EIP1559DenominatorCanyon)
	}
}

func TestChainConfigByOpStackGenesisHash(t *testing.T) {
	hardforkConfigsByName := map[common.Hash]hardforkConfig{
		BobaSepoliaDev0GenesisHash: bobaSepoliaDev0Cfg,
		BobaSepoliaGenesisHash:     bobaSepoliaCfg,
		BobaMainnetGenesisHash:     bobaMainnetCfg,
		BobaBnbTestnetGenesisHash:  bobaBnbTestnetCfg,
		OPSepoliaGenesisHash:       opSepoliaCfg,
		OPMainnetGenesisHash:       opMainnetCfg,
	}

	for genesisHash, expectedHarhardforkCfg := range hardforkConfigsByName {
		gotCfg := ChainConfigByOpStackGenesisHash(genesisHash)
		require.NotNil(t, gotCfg)

		// ChainID
		require.Equal(t, expectedHarhardforkCfg.chainID, gotCfg.ChainID.Uint64())

		// Hardforks
		require.Equal(t, expectedHarhardforkCfg.ShanghaiTime, gotCfg.ShanghaiTime)
		require.Equal(t, expectedHarhardforkCfg.CancunTime, gotCfg.CancunTime)
		require.Equal(t, expectedHarhardforkCfg.BedrockBlock, gotCfg.BedrockBlock)
		require.Equal(t, expectedHarhardforkCfg.RegolithTime, gotCfg.RegolithTime)
		require.Equal(t, expectedHarhardforkCfg.CanyonTime, gotCfg.CanyonTime)
		require.Equal(t, expectedHarhardforkCfg.EcotoneTime, gotCfg.EcotoneTime)
		require.Equal(t, expectedHarhardforkCfg.FjordTime, gotCfg.FjordTime)

		// EIP-1559
		require.Equal(t, expectedHarhardforkCfg.EIP1559Elasticity, gotCfg.Optimism.EIP1559Elasticity)
		require.Equal(t, expectedHarhardforkCfg.EIP1559Denominator, gotCfg.Optimism.EIP1559Denominator)
		require.Equal(t, expectedHarhardforkCfg.EIP1559DenominatorCanyon, gotCfg.Optimism.EIP1559DenominatorCanyon)
	}
}
