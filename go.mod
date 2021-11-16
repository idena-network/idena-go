module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.9.4
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.22.2
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/cosmos/iavl v0.15.3
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/go-stack/stack v1.8.1
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/tink/go v0.0.0-20200401233402-a389e601043a
	github.com/gopherjs/gopherjs v0.0.0-20190910122728-9d188e94fb99 // indirect
	github.com/ipfs/go-blockservice v0.2.0
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-ipfs v0.10.0
	github.com/ipfs/go-ipfs-config v0.16.0
	github.com/ipfs/go-ipfs-files v0.0.9
	github.com/ipfs/go-merkledag v0.5.0
	github.com/ipfs/go-mfs v0.1.2
	github.com/ipfs/go-unixfs v0.3.0
	github.com/ipfs/interface-go-ipfs-core v0.5.2
	github.com/klauspost/compress v1.13.6
	github.com/libp2p/go-libp2p-core v0.9.0
	github.com/libp2p/go-libp2p-pubsub v0.5.6
	github.com/libp2p/go-msgio v0.0.6
	github.com/libp2p/go-yamux v1.4.1
	github.com/mholt/archiver/v3 v3.5.1-0.20210112195346-074da64920d3
	github.com/multiformats/go-multiaddr v0.4.1
	github.com/multiformats/go-multihash v0.0.16
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.8.0
	github.com/shopspring/decimal v0.0.0-20200227202807-02e2044944cc
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	github.com/tendermint/tm-db v0.6.4
	github.com/urfave/cli v1.22.5
	github.com/whyrusleeping/go-logging v0.0.1
	github.com/willf/bitset v1.1.10 // indirect
	github.com/willf/bloom v2.0.3+incompatible
	golang.org/x/crypto v0.0.0-20210813211128-0a44fdfbc16e
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d
	golang.org/x/sys v0.0.0-20210816183151-1e6c022a8912
	google.golang.org/protobuf v1.27.1
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
)

replace github.com/cosmos/iavl => github.com/idena-network/iavl v0.12.3-0.20210604085842-854e73deab29

replace github.com/libp2p/go-libp2p-pnet => github.com/idena-network/go-libp2p-pnet v0.2.1-0.20200406075059-75d9ee9b85ed

go 1.16
