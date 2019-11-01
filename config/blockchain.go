package config

type BlockchainConfig struct {
	// distance between blocks with permanent certificates
	StoreCertRange uint64
	BurnTxRange    uint64
}
