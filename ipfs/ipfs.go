package ipfs

import (
	"bytes"
	"context"
	"fmt"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ipfsConf "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/core/coreunix"
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	dag "github.com/ipfs/go-merkledag"
	dagtest "github.com/ipfs/go-merkledag/test"
	"github.com/ipfs/go-mfs"
	ft "github.com/ipfs/go-unixfs"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/multiformats/go-multihash"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/go-logging"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	CidLength        = 36
	ZeroPeersTimeout = 2 * time.Minute
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
	LoadTo(key []byte, to io.Writer, ctx context.Context, onLoading func(size, loaded int64)) error
	Pin(key []byte) error
	Unpin(key []byte) error
	Cid(data []byte) (cid.Cid, error)
	Port() int
	PeerId() string
	AddFile(absPath string, data io.ReadCloser, fi os.FileInfo) (cid.Cid, error)
}

type ipfsProxy struct {
	node                 *core.IpfsNode
	log                  log.Logger
	cidCache             *cache.Cache
	rwLock               sync.RWMutex
	cfg                  *config.IpfsConfig
	nodeCtx              context.Context
	nodeCtxCancel        context.CancelFunc
	lastPeersUpdatedTime time.Time
}

func NewIpfsProxy(cfg *config.IpfsConfig) (Proxy, error) {
	logging.SetLevel(0, "core")

	err := loadPlugins(cfg)

	if err != nil {
		return nil, err
	}

	logger := log.New()

	node, ctx, cancelCtx, err := createNode(cfg)
	if err != nil {
		return nil, err
	}

	logger.Info("Ipfs initialized", "peerId", node.PeerHost.ID().Pretty())

	c := cache.New(2*time.Minute, 5*time.Minute)
	p := &ipfsProxy{
		node:                 node,
		log:                  logger,
		cfg:                  cfg,
		cidCache:             c,
		nodeCtx:              ctx,
		nodeCtxCancel:        cancelCtx,
		lastPeersUpdatedTime: time.Now().UTC(),
	}

	go p.watchPeers()
	return p, nil
}

func createNode(cfg *config.IpfsConfig) (*core.IpfsNode, context.Context, context.CancelFunc, error) {
	dataDir, _ := filepath.Abs(cfg.DataDir)

	if ln, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.IpfsPort)); err == nil {
		ln.Close()
	} else {
		return nil, nil, func() {}, errors.Errorf("cannot start IPFS node on port %v, err: %v", cfg.IpfsPort, err.Error())
	}

	_, err := configureIpfs(cfg)
	if err != nil {
		return nil, nil, func() {}, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	node, err := core.NewNode(ctx, getNodeConfig(dataDir))
	if err != nil {
		return nil, nil, func() {}, err
	}

	return node, ctx, cancelCtx, nil
}

func (p *ipfsProxy) changePort() {
	p.rwLock.Lock()
	defer p.rwLock.Unlock()

	p.log.Info("Start changing IPFS port", "current", p.cfg.IpfsPort)

	p.nodeCtxCancel()

	c, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	select {
	case <-c.Done():
		p.log.Error("timeout while stopping IPFS")
	case <-p.nodeCtx.Done():
	}

	for {
		p.cfg.IpfsPort += 1

		node, ctx, cancelCtx, err := createNode(p.cfg)

		if err != nil {
			continue
		}

		p.node = node
		p.nodeCtx = ctx
		p.nodeCtxCancel = cancelCtx
		p.log.Info("Finish changing IPFS port", "new", p.cfg.IpfsPort)
		break
	}

}

func (p *ipfsProxy) watchPeers() {
	api, _ := coreapi.NewCoreAPI(p.node)
	logger := log.New("component", "ipfs watch")

	for {
		if !p.cfg.StaticPort && time.Now().UTC().Sub(p.lastPeersUpdatedTime) > ZeroPeersTimeout {
			p.changePort()
			api, _ = coreapi.NewCoreAPI(p.node)
			p.lastPeersUpdatedTime = time.Now().UTC()
		}
		info, err := api.Swarm().Peers(context.Background())
		if err != nil {
			logger.Info("peers info", "err", err)
		}
		if len(info) > 0 {
			p.lastPeersUpdatedTime = time.Now().UTC()
		}
		logger.Trace("last time with non-peers", "time", p.lastPeersUpdatedTime)
		for index, i := range info {
			logger.Trace(strconv.Itoa(index), "id", i.ID().String(), "addr", i.Address().String())
		}
		time.Sleep(time.Second * 10)
	}
}

func (p *ipfsProxy) Add(data []byte) (cid.Cid, error) {
	if len(data) == 0 {
		return EmptyCid, nil
	}

	p.rwLock.RLock()
	defer p.rwLock.RUnlock()
	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	file := files.NewBytesFile(data)
	defer file.Close()
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

func (p *ipfsProxy) AddFile(absPath string, data io.ReadCloser, fi os.FileInfo) (cid.Cid, error) {
	p.rwLock.RLock()
	defer p.rwLock.RUnlock()

	api, _ := coreapi.NewCoreAPI(p.node)

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
	defer cancel()

	file, _ := files.NewReaderPathFile(absPath, data, fi)
	defer file.Close()
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

func (p *ipfsProxy) Get(key []byte) ([]byte, error) {
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

func (p *ipfsProxy) get(path path.Path) ([]byte, error) {
	p.rwLock.RLock()
	defer p.rwLock.RUnlock()
	api, _ := coreapi.NewCoreAPI(p.node)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	f, err := api.Unixfs().Get(ctx, path)
	select {
	case <-ctx.Done():
		err = errors.New("timeout while reading data from ipfs")
	default:
		break
	}
	if err != nil {
		info, _ := api.Swarm().Peers(context.Background())
		p.log.Error("fail to read from ipfs", "cid", path.String(), "err", err, "peers", len(info))
		return nil, err
	}
	file := files.ToFile(f)
	defer file.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(file)

	if err != nil {
		return nil, err
	}
	p.log.Debug("read data from ipfs", "cid", path.String())
	return buf.Bytes(), nil
}

func (p *ipfsProxy) LoadTo(key []byte, to io.Writer, ctx context.Context, onLoading func(size, loaded int64)) error {
	if len(key) == 0 {
		return nil
	}
	c, err := cid.Cast(key)
	if err != nil {
		return err
	}
	if c == EmptyCid {
		return nil
	}

	p.rwLock.RLock()
	defer p.rwLock.RUnlock()
	api, _ := coreapi.NewCoreAPI(p.node)

	f, err := api.Unixfs().Get(ctx, path.IpfsPath(c))
	select {
	case <-ctx.Done():
		return errors.New("ipfs load: context canceled")
	default:
		break
	}
	file := files.ToFile(f)
	defer file.Close()

	size, err := file.Size()
	if err != nil {
		return err
	}
	_, err = io.Copy(to, &progressReader{r: file, size: size, onLoading: onLoading})
	return err
}

func (p *ipfsProxy) Pin(key []byte) error {
	p.rwLock.RLock()
	defer p.rwLock.RUnlock()
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

func (p *ipfsProxy) Unpin(key []byte) error {
	p.rwLock.RLock()
	defer p.rwLock.RUnlock()
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

func (p *ipfsProxy) Port() int {
	return p.cfg.IpfsPort
}

func (p *ipfsProxy) PeerId() string {
	return p.node.PeerHost.ID().Pretty()
}

func (p *ipfsProxy) Cid(data []byte) (cid.Cid, error) {
	if len(data) == 0 {
		return EmptyCid, nil
	}

	hash := rlp.Hash(data)
	cacheKey := string(hash[:])
	if value, ok := p.cidCache.Get(cacheKey); ok {
		return value.(cid.Cid), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nilnode, _ := core.NewNode(ctx, &core.BuildCfg{
		NilRepo: true,
	})
	defer nilnode.Peerstore.Close()

	addblockstore := nilnode.Blockstore
	exch := nilnode.Exchange
	pinning := nilnode.Pinning

	bserv := blockservice.New(addblockstore, exch) // hash security 001
	dserv := dag.NewDAGService(bserv)

	fileAdder, err := coreunix.NewAdder(ctx, pinning, addblockstore, dserv)

	if err != nil {
		return EmptyCid, err
	}

	settings, prefix, err := options.UnixfsAddOptions(options.Unixfs.CidVersion(1))
	fileAdder.Chunker = settings.Chunker
	fileAdder.Pin = false
	fileAdder.RawLeaves = settings.RawLeaves
	fileAdder.CidBuilder = prefix

	md := dagtest.Mock()
	emptyDirNode := ft.EmptyDirNode()
	// Use the same prefix for the "empty" MFS root as for the file adder.
	emptyDirNode.SetCidBuilder(fileAdder.CidBuilder)
	mr, err := mfs.NewRoot(ctx, md, emptyDirNode, nil)
	if err != nil {
		return EmptyCid, err
	}

	fileAdder.SetMfsRoot(mr)
	file := files.NewBytesFile(data)
	defer file.Close()
	nd, err := fileAdder.AddAllAndPin(file)
	if err != nil {
		return EmptyCid, err
	}
	p.cidCache.Set(cacheKey, nd.Cid(), cache.DefaultExpiration)
	return nd.Cid(), nil
}

func configureIpfs(cfg *config.IpfsConfig) (*ipfsConf.Config, error) {
	updateIpfsConfig := func(ipfsConfig *ipfsConf.Config) error {
		ipfsConfig.Addresses.Swarm = []string{
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.IpfsPort),
			fmt.Sprintf("/ip6/::/tcp/%d", cfg.IpfsPort),
		}

		bps, err := ipfsConf.ParseBootstrapPeers(cfg.BootNodes)
		if err != nil {
			return err
		}
		ipfsConfig.Bootstrap = ipfsConf.BootstrapPeerStrings(bps)
		ipfsConfig.Swarm.DisableBandwidthMetrics = true
		ipfsConfig.Routing.Type = cfg.Routing
		ipfsConfig.Swarm.ConnMgr.GracePeriod = cfg.GracePeriod
		ipfsConfig.Swarm.ConnMgr.LowWater = cfg.LowWater
		ipfsConfig.Swarm.ConnMgr.HighWater = cfg.HighWater
		ipfsConfig.Reprovider.Interval = cfg.ReproviderInterval
		return nil
	}
	var ipfsConfig *ipfsConf.Config

	datadir, _ := filepath.Abs(cfg.DataDir)

	if !fsrepo.IsInitialized(datadir) {
		ipfsConfig, err := ipfsConf.Init(os.Stdout, 2048)
		if err != nil {
			return nil, err
		}
		ipfsConfig.Swarm.EnableAutoNATService = true
		ipfsConfig.Swarm.EnableAutoRelay = true
		ipfsConfig.Swarm.EnableRelayHop = true
		ipfsConfig.Experimental.FilestoreEnabled = true

		err = updateIpfsConfig(ipfsConfig)
		if err != nil {
			return nil, err
		}
		if err := fsrepo.Init(datadir, ipfsConfig); err != nil {
			return nil, err
		}

		writeSwarmKey(datadir, cfg.SwarmKey)
	} else {
		ipfsConfig, err := fsrepo.ConfigAt(datadir)
		if err != nil {
			return nil, err
		}
		err = updateIpfsConfig(ipfsConfig)
		if err != nil {
			return nil, err
		}

		repo, err := fsrepo.Open(datadir)
		if err != nil {
			return nil, err
		}
		if err := repo.SetConfig(ipfsConfig); err != nil {
			return nil, err
		}
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

func loadPlugins(cfg *config.IpfsConfig) error {
	dataDir, _ := filepath.Abs(cfg.DataDir)
	pluginPath := filepath.Join(dataDir, "plugins")

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

func (i *memoryIpfs) LoadTo(key []byte, to io.Writer, ctx context.Context, onLoading func(size, loaded int64)) error {
	panic("implement me")
}

func (i *memoryIpfs) AddFile(absPath string, data io.ReadCloser, fi os.FileInfo) (cid.Cid, error) {
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

type progressReader struct {
	r         io.Reader
	read      int
	size      int64
	onLoading func(size, loaded int64)
}

func (r *progressReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if err == nil {
		r.read += n
		if r.onLoading != nil {
			r.onLoading(r.size, int64(r.read))
		}
	}
	return n, err
}
