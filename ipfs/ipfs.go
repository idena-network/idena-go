package ipfs

import (
	"bytes"
	"context"
	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/core/coreapi"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-ipfs-files"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipsn/go-ipfs/plugin/loader"
	"idena-go/log"
	"path/filepath"
)

var DefaultBufSize = 1048576

type Proxy interface {
	Add(data []byte) (cid.Cid, error)
	Get(key []byte) ([]byte, error)
	Pin(key []byte) error
	Cid(data []byte) (cid.Cid, error)
}

type ipfsProxy struct {
	node *core.IpfsNode
	log  log.Logger
}

func NewIpfsProxy(config *IpfsConfig) (Proxy, error) {

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

func (p ipfsProxy) Add(data []byte) (cid.Cid, error) {
	api, _ := coreapi.NewCoreAPI(p.node)

	file := files.NewBytesFile(data)

	path, err := api.Unixfs().Add(context.Background(), file)

	if err != nil {
		return cid.Cid{}, err
	}

	return path.Cid(), nil
}

func (p ipfsProxy) Get(key []byte) ([]byte, error) {
	api, _ := coreapi.NewCoreAPI(p.node)

	c, err := cid.Parse(key)
	if err != nil {
		return nil, err
	}

	f, err := api.Unixfs().Get(context.Background(), iface.IpfsPath(c))

	file := files.ToFile(f)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(file)

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (p ipfsProxy) Pin(key []byte) error {
	api, _ := coreapi.NewCoreAPI(p.node)

	c, err := cid.Parse(key)
	if err != nil {
		return err
	}

	return api.Pin().Add(context.Background(), iface.IpfsPath(c))
}

func (p ipfsProxy) Cid(data []byte) (cid.Cid, error) {
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
