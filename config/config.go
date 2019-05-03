package config

import (
	"crypto/ecdsa"
	"fmt"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/ipfs"
	"idena-go/log"
	"idena-go/p2p"
	"idena-go/p2p/enode"
	"idena-go/p2p/nat"
	"idena-go/rpc"
	"os"
	"path/filepath"
)

const (
	datadirPrivateKey = "nodekey" // Path within the datadir to the node's private key
)

var (
	DefaultBootnode    = "enode://45df5a0a220198ca95e2ee70dea500569e6578eafa603322c028e1e0ba9ca9e40e5810fa3308728d8d8fda565de8e66a4aa5536950128af8ad93de5ac65e8131@127.0.0.1:40404"
	DefaultPort        = 40404
	DefaultRpcHost     = "localhost"
	DefaultRpcPort     = 9009
	DefaultIpfsPort    = 4002
	DefaultNoDiscovery = false
)

type Config struct {
	DataDir     string
	Network     uint32
	Consensus   *ConsensusConf
	P2P         *p2p.Config
	RPC         *rpc.Config
	GenesisConf *GenesisConf
	IpfsConf    *ipfs.IpfsConfig
	Validation  *ValidationConfig
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

// ResolvePath resolves path in the instance directory.
func (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, path)
}

func (c *Config) KeyStoreDataDir() (string, error) {
	instanceDir := filepath.Join(c.DataDir, "keystore")
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		log.Error(fmt.Sprintf("Failed to create keystore datadir: %v", err))
		return "", err
	}
	return instanceDir, nil
}

func GetDefaultConfig(datadir string, port int, automine bool, rpcaddr string, rpcport int, bootstrap string, ipfsBootstrap string, ipfsPort int, noDiscovery bool, godAddress string) *Config {
	var nodes []*enode.Node
	if bootstrap != "" {
		p, err := enode.ParseV4(bootstrap)
		if err == nil {
			nodes = append(nodes, p)
		} else {
			log.Warn("Cant parse bootstrap node")
		}
	}

	c := Config{
		DataDir: datadir,
		P2P: &p2p.Config{
			ListenAddr:     fmt.Sprintf(":%d", port),
			MaxPeers:       25,
			NAT:            nat.Any(),
			BootstrapNodes: nodes,
			NoDiscovery:    noDiscovery,
		},
		Consensus: GetDefaultConsensusConfig(automine),
		RPC:       rpc.GetDefaultRPCConfig(rpcaddr, rpcport),
		GenesisConf: &GenesisConf{
			GodAddress: common.HexToAddress(godAddress),
		},
		IpfsConf:   ipfs.GetDefaultIpfsConfig(datadir, ipfsPort, ipfsBootstrap),
		Validation: GetDefaultValidationConfig(),
	}

	return &c
}
