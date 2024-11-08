package networkname

const (
	MainnetChainName        = "mainnet"
	HoleskyChainName        = "holesky"
	SepoliaChainName        = "sepolia"
	GoerliChainName         = "goerli"
	DevChainName            = "dev"
	MumbaiChainName         = "mumbai"
	AmoyChainName           = "amoy"
	BorMainnetChainName     = "bor-mainnet"
	BorDevnetChainName      = "bor-devnet"
	GnosisChainName         = "gnosis"
	BorE2ETestChain2ValName = "bor-e2e-test-2Val"
	ChiadoChainName         = "chiado"

	OPMainnetChainName = "op-mainnet"
	OPSepoliaChainName = "op-sepolia"

	BobaMainnetChainName = "boba-mainnet"
	BobaSepoliaChainName = "boba-sepolia"

	LegacyOPMainnetChainName = "optimism-mainnet"
	LegacyOPSepoliaChainName = "optimism-sepolia"
)

var All = []string{
	MainnetChainName,
	HoleskyChainName,
	SepoliaChainName,
	GoerliChainName,
	MumbaiChainName,
	AmoyChainName,
	BorMainnetChainName,
	BorDevnetChainName,
	GnosisChainName,
	ChiadoChainName,

	OPMainnetChainName,
	OPSepoliaChainName,

	BobaMainnetChainName,
	BobaSepoliaChainName,
}

func HandleLegacyName(name string) string {
	switch name {
	case LegacyOPSepoliaChainName:
		return OPSepoliaChainName
	case LegacyOPMainnetChainName:
		return OPMainnetChainName
	default:
		return name
	}
}
