package config

import (
	"crypto/ecdsa"
	"fmt"
	"idena-go/crypto"
	"idena-go/log"
	"idena-go/p2p"
	"os"
	"path/filepath"
)


const (
	datadirPrivateKey      = "nodekey"            // Path within the datadir to the node's private key
)

type Config struct{
	DataDir string
	Network int32
	Consensus *ConsensusConf
	P2P *p2p.Config
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
func  (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, path)
}