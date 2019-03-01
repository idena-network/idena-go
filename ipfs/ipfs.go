package ipfs

import (
	"context"
	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/ipsn/go-ipfs/plugin/loader"
	"idena-go/log"
	"path/filepath"
)

type IpfsProxy interface {
	Add(data []byte) (cid.Cid, error)
	Get(key []byte) ([]byte, error)
	Pin(key []byte) error
	Cid(data []byte) (cid.Cid, error)
}

type ipfsProxy struct {
	node *core.IpfsNode
	log  log.Logger
}

func NewIpfsProxy(config *IpfsConfig) (IpfsProxy, error) {

	loadPlugins(config.datadir)

	if err := config.Initialize(); err != nil {
		return nil, err
	}

	logger := log.New()

	node, err := core.NewNode(context.Background(), GetNodeConfig(config.datadir))

	if err != nil {
		return nil, err
	}

	logger.Info("Ipfs initialized", "peerId", node.PeerHost.ID().Pretty())

	return &ipfsProxy{
		node: node,
		log:  logger,
	}, nil
}

func (ipfsProxy) Add(data []byte) (cid.Cid, error) {
	panic("implement me")
}

func (ipfsProxy) Get(key []byte) ([]byte, error) {
	panic("implement me")
}

func (ipfsProxy) Pin(key []byte) error {
	panic("implement me")
}

func (ipfsProxy) Cid(data []byte) (cid.Cid, error) {
	format := cid.V0Builder{}
	return format.Sum(data)
}

func loadPlugins(ipfsPath string) (*loader.PluginLoader, error) {
	pluginpath := filepath.Join(ipfsPath, "plugins")

	var plugins *loader.PluginLoader
	plugins, err := loader.NewPluginLoader(pluginpath)

	if err != nil {
		log.Error("ipfs plugin loader error")
	}

	if err := plugins.Initialize(); err != nil {
		log.Error("ipfs plugin initialization error")
	}

	if err := plugins.Inject(); err != nil {
		log.Error("ipfs plugin inject error")
	}

	return plugins, nil
}
