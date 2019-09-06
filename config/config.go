package config

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/p2p"
	"github.com/idena-network/idena-go/p2p/enode"
	"github.com/idena-network/idena-go/p2p/nat"
	"github.com/idena-network/idena-go/rpc"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	datadirPrivateKey = "nodekey" // Path within the datadir to the node's private key
)

type Config struct {
	DataDir          string
	Network          uint32
	Consensus        *ConsensusConf
	P2P              *p2p.Config
	RPC              *rpc.Config
	GenesisConf      *GenesisConf
	IpfsConf         *IpfsConfig
	Validation       *ValidationConfig
	Sync             *SyncConfig
	OfflineDetection *OfflineDetectionConfig
	Blockchain       *BlockchainConfig
}

func (c *Config) ProvideNodeKey(key string, password string) error {
	instanceDir := filepath.Join(c.DataDir, "keystore")
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		return err
	}

	keyfile := filepath.Join(instanceDir, datadirPrivateKey)
	if _, err := crypto.LoadECDSA(keyfile); err == nil {
		return errors.New("key already exists")
	}

	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return errors.Errorf("error while decoding key, err: %v", err.Error())
	}

	decrypted, err := crypto.Decrypt(keyBytes, password)
	if err != nil {
		return errors.Errorf("error while decrypting key, err: %v", err.Error())
	}

	ecdsaKey, err := crypto.ToECDSA(decrypted)
	if err != nil {
		return errors.Errorf("key is not valid ECDSA key, err: %v", err.Error())
	}
	if err := crypto.SaveECDSA(keyfile, ecdsaKey); err != nil {
		return errors.Errorf("failed to persist key, err: %v", err.Error())
	}
	return nil
}

func (c *Config) NodeKey() *ecdsa.PrivateKey {
	// Use any specifically configured key.
	if c.P2P.PrivateKey != nil {
		return c.P2P.PrivateKey
	}
	// Generate ephemeral key if no datadir is being used.
	if c.DataDir == "" {
		key, err := crypto.GenerateKey()
		if err != nil {
			log.Crit(fmt.Sprintf("Failed to generate ephemeral node key: %v", err))
		}
		return key
	}

	instanceDir := filepath.Join(c.DataDir, "keystore")
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
		return nil
	}

	keyfile := filepath.Join(instanceDir, datadirPrivateKey)
	if key, err := crypto.LoadECDSA(keyfile); err == nil {
		return key
	}
	// No persistent key found, generate and store a new one.
	key, err := crypto.GenerateKey()
	if err != nil {
		log.Crit(fmt.Sprintf("Failed to generate node key: %v", err))
	}
	if err := crypto.SaveECDSA(keyfile, key); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
	}
	return key
}

// NodeDB returns the path to the discovery node database.
func (c *Config) NodeDB() string {
	if c.DataDir == "" {
		return "" // ephemeral
	}
	return filepath.Join(c.DataDir, "nodes")
}

func (c *Config) KeyStoreDataDir() (string, error) {
	instanceDir := filepath.Join(c.DataDir, "keystore")
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		log.Error(fmt.Sprintf("Failed to create keystore datadir: %v", err))
		return "", err
	}
	return instanceDir, nil
}

func MakeMobileConfig(path string, cfg string) (*Config, error) {
	conf := getDefaultConfig(filepath.Join(path, DefaultDataDir))

	if cfg != "" {
		log.Info("using custom configuration")
		bytes := []byte(cfg)
		err := json.Unmarshal(bytes, &conf)
		if err != nil {
			return nil, errors.Errorf("Cannot parse JSON config")
		}
	} else {
		log.Info("using default config")
	}

	return conf, nil
}

func MakeConfig(ctx *cli.Context) (*Config, error) {
	cfg, err := MakeConfigFromFile(ctx.String(CfgFileFlag.Name))
	if err != nil {
		return nil, err
	}
	applyFlags(ctx, cfg)
	return cfg, nil
}

func MakeConfigFromFile(file string) (*Config, error) {
	cfg := getDefaultConfig(DefaultDataDir)
	if file != "" {
		if err := loadConfig(file, cfg); err != nil {
			log.Error(err.Error())
			return nil, err
		}
	}
	return cfg, nil
}

func getDefaultConfig(dataDir string) *Config {
	var bootNodes []*enode.Node
	for _, item := range DefaultBootstrapNodes {
		bootNode, _ := enode.ParseV4(item)
		bootNodes = append(bootNodes, bootNode)
	}

	return &Config{
		DataDir: dataDir,
		Network: 0x1, // testnet
		P2P: &p2p.Config{
			ListenAddr:     fmt.Sprintf(":%d", DefaultPort),
			MaxPeers:       50,
			NAT:            nat.Any(),
			BootstrapNodes: bootNodes,
		},
		Consensus: GetDefaultConsensusConfig(),
		RPC:       rpc.GetDefaultRPCConfig(DefaultRpcHost, DefaultRpcPort),
		GenesisConf: &GenesisConf{
			FirstCeremonyTime: DefaultCeremonyTime,
			GodAddress:        common.HexToAddress(DefaultGodAddress),
		},
		IpfsConf: &IpfsConfig{
			DataDir:   filepath.Join(dataDir, DefaultIpfsDataDir),
			IpfsPort:  DefaultIpfsPort,
			BootNodes: DefaultIpfsBootstrapNodes,
			SwarmKey:  DefaultSwarmKey,
		},
		Validation: &ValidationConfig{},
		Sync: &SyncConfig{
			FastSync:      true,
			ForceFullSync: DefaultForceFullSync,
		},
		OfflineDetection: GetDefaultOfflineDetectionConfig(),
		Blockchain: &BlockchainConfig{
			StoreCertRange: DefaultStoreCertRange,
		},
	}
}

func applyFlags(ctx *cli.Context, cfg *Config) {
	if ctx.IsSet(DataDirFlag.Name) {
		cfg.DataDir = ctx.String(DataDirFlag.Name)
	}

	applyP2PFlags(ctx, cfg)
	applyConsensusFlags(ctx, cfg)
	applyRpcFlags(ctx, cfg)
	applyGenesisFlags(ctx, cfg)
	applyIpfsFlags(ctx, cfg)
	applyValidationFlags(ctx, cfg)
	applySyncFlags(ctx, cfg)
}

func applySyncFlags(ctx *cli.Context, cfg *Config) {
	if ctx.IsSet(FastSyncFlag.Name) {
		cfg.Sync.FastSync = ctx.Bool(FastSyncFlag.Name)
	}
	if ctx.IsSet(ForceFullSyncFlag.Name) {
		cfg.Sync.ForceFullSync = ctx.Uint64(ForceFullSyncFlag.Name)
	}
}

func applyP2PFlags(ctx *cli.Context, cfg *Config) {

	if ctx.IsSet(BootNodeFlag.Name) {
		var nodes []*enode.Node
		p, err := enode.ParseV4(ctx.String(BootNodeFlag.Name))
		if err == nil {
			nodes = append(nodes, p)
		} else {
			log.Warn("Cant parse bootstrap node")
		}
		cfg.P2P.BootstrapNodes = nodes
	}
	cfg.P2P.NodeDatabase = cfg.NodeDB()

	if ctx.IsSet(TcpPortFlag.Name) {
		cfg.P2P.ListenAddr = fmt.Sprintf(":%d", ctx.Int(TcpPortFlag.Name))
	}
	if ctx.IsSet(NoDiscoveryFlag.Name) {
		cfg.P2P.NoDiscovery = ctx.Bool(NoDiscoveryFlag.Name)
	}
	if ctx.IsSet(MaxNetworkDelayFlag.Name) {
		cfg.P2P.MaxDelay = ctx.Int(MaxNetworkDelayFlag.Name)
	}
}

func applyConsensusFlags(ctx *cli.Context, cfg *Config) {
	if ctx.IsSet(AutomineFlag.Name) {
		cfg.Consensus.Automine = ctx.Bool(AutomineFlag.Name)
	}
}

func applyRpcFlags(ctx *cli.Context, cfg *Config) {
	if ctx.IsSet(RpcHostFlag.Name) {
		cfg.RPC.HTTPHost = ctx.String(RpcHostFlag.Name)
	}
	if ctx.IsSet(RpcPortFlag.Name) {
		cfg.RPC.HTTPPort = ctx.Int(RpcPortFlag.Name)
	}
}

func applyGenesisFlags(ctx *cli.Context, cfg *Config) {
	if ctx.IsSet(GodAddressFlag.Name) {
		cfg.GenesisConf.GodAddress = common.HexToAddress(ctx.String(GodAddressFlag.Name))
	}
	if ctx.IsSet(CeremonyTimeFlag.Name) {
		cfg.GenesisConf.FirstCeremonyTime = ctx.Int64(CeremonyTimeFlag.Name)
	}
}

func applyIpfsFlags(ctx *cli.Context, cfg *Config) {
	cfg.IpfsConf.DataDir = filepath.Join(cfg.DataDir, DefaultIpfsDataDir)

	if ctx.IsSet(IpfsPortFlag.Name) {
		cfg.IpfsConf.IpfsPort = ctx.Int(IpfsPortFlag.Name)
	}

	if ctx.IsSet(IpfsBootNodeFlag.Name) {
		cfg.IpfsConf.BootNodes = []string{ctx.String(IpfsBootNodeFlag.Name)}
	}
}

func applyValidationFlags(ctx *cli.Context, cfg *Config) {

}

func loadConfig(configPath string, conf *Config) error {
	if _, err := os.Stat(configPath); err != nil {
		return errors.Errorf("Config file cannot be found, path: %v", configPath)
	}

	if jsonFile, err := os.Open(configPath); err != nil {
		return errors.Errorf("Config file cannot be opened, path: %v", configPath)
	} else {
		byteValue, _ := ioutil.ReadAll(jsonFile)
		err := json.Unmarshal(byteValue, &conf)
		if err != nil {
			return errors.Errorf("Cannot parse JSON config, path: %v", configPath)
		}
		return nil
	}
}
