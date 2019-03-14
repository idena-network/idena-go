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
	"idena-go/rlp"
	"path/filepath"
	"strconv"
	"time"
)

var (
	DefaultBufSize = 1048576
	EmptyCid       cid.Cid
)

func init() {
	e, _ := cid.Decode("QmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n")
	EmptyCid = e
}

type Proxy interface {
	Add(data []byte) (cid.Cid, error)
	AddDirectory(data map[string][]byte, hashOnly bool) (cid.Cid, error)
	GetDirectory(key []byte) (map[string][]byte, error)
	Get(key []byte) ([]byte, error)
	GetFile(directoryKey []byte, filename string) ([]byte, error)
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
	node.Repo.SetConfig(config.cfg)
	logger.Info("Ipfs initialized", "peerId", node.PeerHost.ID().Pretty())
	go watchPeers(node)
	return &ipfsProxy{
		node: node,
		log:  logger,
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
			logger.Info(strconv.Itoa(index), "id", i.ID().String(), "addr", i.Address().String())
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
	path, err := api.Unixfs().Add(ctx, file)
	select {
	case <-ctx.Done():
		err = errors.New("timeout while writing data to ipfs")
	default:
		break
	}
	if err != nil {
		return cid.Cid{}, err
	}
	p.log.Info("Add ipfs data", "cid", path.Cid().String())
	return path.Cid(), nil
}

func (p ipfsProxy) AddDirectory(data map[string][]byte, hashOnly bool) (cid.Cid, error) {

	if len(data) == 0 {
		return EmptyCid, nil
	}
	nodes := make(map[string]files.Node)
	for key, value := range data {
		nodes[key] = files.NewBytesFile(value)
	}
	dir := files.NewMapDirectory(nodes)
	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	path, err := api.Unixfs().Add(ctx, dir, options.Unixfs.HashOnly(hashOnly), options.Unixfs.Wrap(true))
	select {
	case <-ctx.Done():
		err = errors.New("timeout while writing directory to ipfs")
	default:
		break
	}
	if err != nil {
		return cid.Cid{}, err
	}
	p.log.Info("Add ipfs data dir", "cid", path.Cid().String())
	return path.Cid(), nil
}

func (p ipfsProxy) GetDirectory(key []byte) (map[string][]byte, error) {
	c, err := cid.Cast(key)
	if err != nil {
		return nil, err
	}
	if c == EmptyCid {
		return make(map[string][]byte), nil
	}
	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()

	// download entire directory
	_, err = api.Unixfs().Get(ctx, iface.IpfsPath(c))

	if err != nil {
		p.log.Error("fail to read from ipfs", "cid", c.String(), "err", err)
		return nil, err
	}

	links, err := api.Unixfs().Ls(ctx, iface.IpfsPath(c))

	if err != nil {
		p.log.Error("fail to read from ipfs", "cid", c.String(), "err", err)
		return nil, err
	}
	time.Sleep(time.Second * 31)
	result := make(map[string][]byte)
	for link := range links {
		if link.Type != iface.TFile {
			continue
		}
		f, err := api.Unixfs().Get(ctx, iface.IpfsPath(link.Cid))
		if err != nil {
			p.log.Error("fail to read from ipfs", "cid", c.String(), "err", err)
			return nil, err
		}
		file := files.ToFile(f)

		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(file)

		if err != nil {
			return nil, err
		}
		result[link.Name] = buf.Bytes()
	}

	select {
	case <-ctx.Done():
		return nil, errors.New("timeout while reading directory from ipfs")
	default:
		break
	}
	return result, nil
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

func (p ipfsProxy) GetFile(directoryKey []byte, filename string) ([]byte, error) {
	c, err := cid.Cast(directoryKey)
	if err != nil {
		return nil, err
	}
	if c == EmptyCid {
		return []byte{}, nil
	}

	return p.get(iface.Join(iface.IpfsPath(c), filename))
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
	p.log.Info("read data from ipfs", "cid", path.String())
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

func (i memoryIpfs) GetFile(directoryKey []byte, filename string) ([]byte, error) {
	dir, err := i.GetDirectory(directoryKey)
	if err != nil {
		return nil, err
	}

	return dir[filename], nil
}

func (i memoryIpfs) GetDirectory(key []byte) (map[string][]byte, error) {
	c, err := cid.Parse(key)
	if err != nil {
		return nil, err
	}
	if v, ok := i.values[c]; ok {
		m := make(map[string][]byte)
		rlp.DecodeBytes(v, &m)
		return m, nil
	}
	return nil, errors.New("not found")
}

func (i memoryIpfs) AddDirectory(data map[string][]byte, hashOnly bool) (cid.Cid, error) {
	enc, _ := rlp.EncodeToBytes(data)
	cid, _ := i.Cid(enc)
	if !hashOnly {
		i.values[cid] = enc
	}
	return cid, nil
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

func (memoryIpfs) Cid(data []byte) (cid.Cid, error) {
	format := cid.V0Builder{}
	return format.Sum(data)
}
