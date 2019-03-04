package ipfs

import (
	"fmt"
	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-ipfs-config"
	"github.com/ipsn/go-ipfs/repo/fsrepo"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const swarmKeyFile = "swarm.key"
const swarmKey = "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530"

type IpfsConfig struct {
	cfg       *config.Config
	datadir   string
	bootnodes []config.BootstrapPeer
}

func GetDefaultIpfsConfig(datadir string, ipfsPort int, bootstrap string) *IpfsConfig {
	var bps []config.BootstrapPeer

	if bootstrap != "" {
		b := strings.Split(bootstrap, ";")
		for _, item := range b {
			peer, err := config.ParseBootstrapPeer(item)
			if err != nil {
				continue
			}
			bps = append(bps, peer)
		}
	}

	ipfsConfig, _ := config.Init(os.Stdout, 2048)

	ipfsConfig.Swarm.EnableAutoNATService = true
	ipfsConfig.Swarm.EnableAutoRelay = true
	ipfsConfig.Swarm.EnableRelayHop = true
	ipfsConfig.Addresses.Swarm = []string{
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", ipfsPort),
		fmt.Sprintf("/ip6/::/tcp/%d", ipfsPort),
	}
	ipfsConfig.Bootstrap = []string{}

	return &IpfsConfig{
		cfg:       ipfsConfig,
		datadir:   filepath.Join(datadir, "ipfs"),
		bootnodes: bps,
	}
}

func (c *IpfsConfig) Initialize() error {
	if !fsrepo.IsInitialized(c.datadir) {

		if err := fsrepo.Init(c.datadir, c.cfg); err != nil {
			return err
		}

		return writeSwarmKey(c.datadir)
	} else {
		current, _ := fsrepo.ConfigAt(c.datadir)

		//if port was changed
		current.Addresses.Swarm = c.cfg.Addresses.Swarm

		// if bootnodes were changed
		current.SetBootstrapPeers(c.bootnodes)

		c.cfg = current
	}
	return nil
}

func writeSwarmKey(datadir string) error {
	spath := filepath.Join(datadir, swarmKeyFile)
	return ioutil.WriteFile(spath, []byte(fmt.Sprintf("/key/swarm/psk/1.0.0/\n/base16/\n%v", swarmKey)), 0644)
}

func GetNodeConfig(datadir string) *core.BuildCfg {
	repo, _ := fsrepo.Open(datadir)

	return &core.BuildCfg{
		Repo:                        repo,
		Permanent:                   true,
		Online:                      true,
		DisableEncryptedConnections: false,
		ExtraOpts: map[string]bool{
			"pubsub": false,
			"ipnsps": false,
			"mplex":  false,
		},
	}
}
