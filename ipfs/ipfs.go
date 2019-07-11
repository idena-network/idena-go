package ipfs

import (
	"bytes"
	"context"
	"fmt"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/log"
	"github.com/ipfs/go-cid"
	ipfsConf "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"os"
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
	e, _ := cid.Decode("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")
	EmptyCid = e

	for i := range MaxCid {
		MaxCid[i] = 0xFF
	}
}

type Proxy interface {
	Add(data []byte) (cid.Cid, error)
	Get(key []byte) ([]byte, error)
	LoadTo(key []byte, to io.Writer, ctx context.Context)
	Pin(key []byte) error
	Unpin(key []byte) error
	Cid(data []byte) (cid.Cid, error)
	Port() int
	PeerId() string
	AddFile(data io.Reader) (cid.Cid, error)
}

type ipfsProxy struct {
	node   *core.IpfsNode
	log    log.Logger
	port   int
	peerId string
}

func (p ipfsProxy) LoadTo(key []byte, to io.Writer, ctx context.Context) {
	panic("implement me")
}

func NewIpfsProxy(cfg *config.IpfsConfig) (Proxy, error) {

	err := loadPlugins(cfg.DataDir)

	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	ipfsConfig, err := configureIpfs(cfg)
	if err != nil {
		return nil, err
	}

	logger := log.New()

	node, err := core.NewNode(context.Background(), getNodeConfig(cfg.DataDir))
	if err != nil {
		return nil, err
	}
	node.Repo.SetConfig(ipfsConfig)
	peerId := node.PeerHost.ID().Pretty()
	logger.Info("Ipfs initialized", "peerId", peerId)
	go watchPeers(node)
	return &ipfsProxy{
		node:   node,
		log:    logger,
		peerId: peerId,
		port:   cfg.IpfsPort,
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
	path, err := api.Unixfs().Add(ctx, file, options.Unixfs.Pin(true), options.Unixfs.CidVersion(1))
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

func (p ipfsProxy) AddFile(data io.Reader) (cid.Cid, error) {

	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
	defer cancel()

	file := files.NewReaderFile(data)
	path, err := api.Unixfs().Add(ctx, file, options.Unixfs.Nocopy(true), options.Unixfs.CidVersion(1))
	select {
	case <-ctx.Done():
		err = errors.New("timeout while writing data to ipfs from reader")
	default:
		break
	}
	if err != nil {
		return cid.Cid{}, err
	}
	p.log.Debug("Add ipfs data from reader", "cid", path.Cid().String())
	return path.Cid(), nil
}

func (p ipfsProxy) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return []byte{}, nil
	}
	c, err := cid.Cast(key)
	if err != nil {
		return nil, err
	}
	if c == EmptyCid {
		return []byte{}, nil
	}
	return p.get(path.IpfsPath(c))
}

func (p ipfsProxy) get(path path.Path) ([]byte, error) {
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

	err = api.Pin().Add(ctx, path.IpfsPath(c))

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

	err = api.Pin().Rm(ctx, path.IpfsPath(c))

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
	var v1CidPrefix = cid.Prefix{
		Codec:    cid.Raw,
		MhLength: -1,
		MhType:   multihash.SHA2_256,
		Version:  1,
	}
	return v1CidPrefix.Sum(data)
}

func configureIpfs(cfg *config.IpfsConfig) (*ipfsConf.Config, error) {
	updateIpfsConfig := func(ipfsConfig *ipfsConf.Config) {
		ipfsConfig.Addresses.Swarm = []string{
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.IpfsPort),
			fmt.Sprintf("/ip6/::/tcp/%d", cfg.IpfsPort),
		}

		bps, _ := ipfsConf.ParseBootstrapPeers(cfg.BootNodes)
		ipfsConfig.Bootstrap = ipfsConf.BootstrapPeerStrings(bps)
	}
	var ipfsConfig *ipfsConf.Config
	if !fsrepo.IsInitialized(cfg.DataDir) {
		ipfsConfig, _ = ipfsConf.Init(os.Stdout, 2048)

		ipfsConfig.Swarm.EnableAutoNATService = true
		ipfsConfig.Swarm.EnableAutoRelay = true
		ipfsConfig.Swarm.EnableRelayHop = true

		updateIpfsConfig(ipfsConfig)

		if err := fsrepo.Init(cfg.DataDir, ipfsConfig); err != nil {
			return nil, err
		}

		writeSwarmKey(cfg.DataDir, cfg.SwarmKey)
	} else {
		ipfsConfig, _ = fsrepo.ConfigAt(cfg.DataDir)

		updateIpfsConfig(ipfsConfig)
	}
	return ipfsConfig, nil
}

func writeSwarmKey(dataDir string, swarmKey string) {
	swarmPath := filepath.Join(dataDir, "swarm.key")
	if _, err := os.Stat(swarmPath); os.IsNotExist(err) {
		err = ioutil.WriteFile(swarmPath, []byte(fmt.Sprintf("/key/swarm/psk/1.0.0/\n/base16/\n%v", swarmKey)), 0644)
		if err != nil {
			log.Error(fmt.Sprintf("Failed to persist swarm file: %v", err))
		}
	}
}

func getNodeConfig(dataDir string) *core.BuildCfg {
	repo, _ := fsrepo.Open(dataDir)

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

func loadPlugins(ipfsPath string) error {
	pluginPath := filepath.Join(ipfsPath, "plugins")

	var plugins *loader.PluginLoader
	plugins, err := loader.NewPluginLoader(pluginPath)

	if err != nil {
		return errors.New("ipfs plugin loader error")
	}

	if err := plugins.Initialize(); err != nil {
		return errors.New("ipfs plugin initialization error")
	}

	if err := plugins.Inject(); err != nil {
		return errors.New("ipfs plugin inject error")
	}

	return nil
}

func NewMemoryIpfsProxy() Proxy {
	return &memoryIpfs{
		values: make(map[cid.Cid][]byte),
	}
}

type memoryIpfs struct {
	values map[cid.Cid][]byte
}

func (i *memoryIpfs) LoadTo(key []byte, to io.Writer, ctx context.Context) {
	panic("implement me")
}

func (i *memoryIpfs) AddFile(data io.Reader) (cid.Cid, error) {
	panic("implement me")
}

func (i *memoryIpfs) Unpin(key []byte) error {
	return nil
}

func (i *memoryIpfs) Add(data []byte) (cid.Cid, error) {
	cid, _ := i.Cid(data)
	i.values[cid] = data
	return cid, nil
}

func (i *memoryIpfs) Get(key []byte) ([]byte, error) {
	c, err := cid.Parse(key)
	if err != nil {
		return nil, err
	}
	if v, ok := i.values[c]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func (*memoryIpfs) Pin(key []byte) error {
	return nil
}

func (*memoryIpfs) PeerId() string {
	return ""
}

func (*memoryIpfs) Port() int {
	return 0
}

func (*memoryIpfs) Cid(data []byte) (cid.Cid, error) {
	var v1CidPrefix = cid.Prefix{
		Codec:    cid.Raw,
		MhLength: -1,
		MhType:   multihash.SHA2_256,
		Version:  1,
	}
	return v1CidPrefix.Sum(data)
}
