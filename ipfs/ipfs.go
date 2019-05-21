package ipfs

import (
	"bytes"
	"context"
	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/core/coreapi"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-ipfs-files"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipsn/go-ipfs/plugin/loader"
	"github.com/pkg/errors"
	"idena-go/log"
	"path/filepath"
	"strconv"
	"time"
)

const (
	DefaultBufSize = 1048576
	CidLength      = 34
)

var (
	EmptyCid cid.Cid
	MinCid   [CidLength]byte
	MaxCid   [CidLength]byte
)

func init() {
	e, _ := cid.Decode("QmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n")
	EmptyCid = e

	for i := range MaxCid {
		MaxCid[i] = 0xFF
	}
}

type Proxy interface {
	Add(data []byte) (cid.Cid, error)
	Get(key []byte) ([]byte, error)
	Pin(key []byte) error
	Unpin(key []byte) error
	Cid(data []byte) (cid.Cid, error)
	Port() int
	PeerId() string
}

type ipfsProxy struct {
	node   *core.IpfsNode
	log    log.Logger
	port   int
	peerId string
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
	node.Repo.SetConfig(config.cfg)
	peerId := node.PeerHost.ID().Pretty()
	logger.Info("Ipfs initialized", "peerId", peerId)
	go watchPeers(node)
	return &ipfsProxy{
		node:   node,
		log:    logger,
		peerId: peerId,
		port:   config.ipfsPort,
	}, nil
}

func watchPeers(node *core.IpfsNode) {
	api, _ := coreapi.NewCoreAPI(node)
	logger := log.New("component", "ipfs watch")

	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		info, err := api.Swarm().Peers(ctx)
		cancel()
		if err != nil {
			logger.Info("peers info", "err", err)
		}
		for index, i := range info {
			logger.Trace(strconv.Itoa(index), "id", i.ID().String(), "addr", i.Address().String())
		}
		time.Sleep(time.Second * 10)
	}
}

func (p ipfsProxy) Add(data []byte) (cid.Cid, error) {
	if len(data) == 0 {
		return EmptyCid, nil
	}
	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	file := files.NewBytesFile(data)
	path, err := api.Unixfs().Add(ctx, file, options.Unixfs.Pin(true))
	select {
	case <-ctx.Done():
		err = errors.New("timeout while writing data to ipfs")
	default:
		break
	}
	if err != nil {
		return cid.Cid{}, err
	}
	p.log.Debug("Add ipfs data", "cid", path.Cid().String())
	return path.Cid(), nil
}

func (p ipfsProxy) Get(key []byte) ([]byte, error) {
	c, err := cid.Cast(key)
	if err != nil {
		return nil, err
	}
	if c == EmptyCid {
		return []byte{}, nil
	}
	return p.get(iface.IpfsPath(c))
}

func (p ipfsProxy) get(path iface.Path) ([]byte, error) {
	api, _ := coreapi.NewCoreAPI(p.node)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	f, err := api.Unixfs().Get(ctx, path)

	select {
	case <-ctx.Done():
		err = errors.New("timeout while reading data from ipfs")
	default:
		break
	}

	if err != nil {
		p.log.Error("fail to read from ipfs", "cid", path.String(), "err", err)
		return nil, err
	}

	file := files.ToFile(f)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(file)

	if err != nil {
		return nil, err
	}
	p.log.Debug("read data from ipfs", "cid", path.String())
	return buf.Bytes(), nil
}

func (p ipfsProxy) Pin(key []byte) error {
	api, _ := coreapi.NewCoreAPI(p.node)

	c, err := cid.Cast(key)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	err = api.Pin().Add(ctx, iface.IpfsPath(c))

	select {
	case <-ctx.Done():
		err = errors.Errorf("timeout while pinning data to ipfs, key: %v", c.String())
	default:
		break
	}

	return err
}

func (p ipfsProxy) Unpin(key []byte) error {
	api, _ := coreapi.NewCoreAPI(p.node)

	c, err := cid.Cast(key)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	err = api.Pin().Rm(ctx, iface.IpfsPath(c))

	select {
	case <-ctx.Done():
		err = errors.Errorf("timeout while unpin data from ipfs, key: %v", c.String())
	default:
		break
	}

	return err
}

func (p ipfsProxy) Port() int {
	return p.port
}

func (p ipfsProxy) PeerId() string {
	return p.peerId
}

func (p ipfsProxy) Cid(data []byte) (cid.Cid, error) {

	if len(data) == 0 {
		return EmptyCid, nil
	}

	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	file := files.NewBytesFile(data)
	path, err := api.Unixfs().Add(ctx, file, options.Unixfs.HashOnly(true))

	select {
	case <-ctx.Done():
		err = errors.New("timeout while getting cid")
	default:
		break
	}

	return path.Cid(), err
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

func NewMemoryIpfsProxy() Proxy {
	return &memoryIpfs{
		values: make(map[cid.Cid][]byte),
	}
}

type memoryIpfs struct {
	values map[cid.Cid][]byte
}

func (i memoryIpfs) Unpin(key []byte) error {
	return nil
}

func (i memoryIpfs) Add(data []byte) (cid.Cid, error) {
	cid, _ := i.Cid(data)
	i.values[cid] = data
	return cid, nil
}

func (i memoryIpfs) Get(key []byte) ([]byte, error) {
	c, err := cid.Parse(key)
	if err != nil {
		return nil, err
	}
	if v, ok := i.values[c]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func (memoryIpfs) Pin(key []byte) error {
	return nil
}

func (memoryIpfs) PeerId() string {
	return ""
}

func (memoryIpfs) Port() int {
	return 0
}

func (memoryIpfs) Cid(data []byte) (cid.Cid, error) {
	format := cid.V0Builder{}
	return format.Sum(data)
}
