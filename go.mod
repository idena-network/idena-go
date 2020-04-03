module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.4.18
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.22.1
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/go-stack/stack v1.8.0
	github.com/golang/mock v1.3.1 // indirect
	github.com/golang/snappy v0.0.1
	github.com/google/tink v1.2.2
	github.com/ipfs/go-blockservice v0.1.2
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-ipfs v0.4.22-0.20200207225350-45efea8aef15
	github.com/ipfs/go-ipfs-config v0.2.0
	github.com/ipfs/go-ipfs-files v0.0.6
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipfs/go-mfs v0.1.1
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipfs/interface-go-ipfs-core v0.2.5
	github.com/jteeuwen/go-bindata v3.0.7+incompatible
	github.com/libp2p/go-libp2p-core v0.3.0
	github.com/libp2p/go-libp2p-pnet v0.1.1-0.20200204202027-d227cde94463 // indirect
	github.com/libp2p/go-msgio v0.0.4
	github.com/libp2p/go-yamux v1.2.3
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/multiformats/go-multiaddr v0.2.0
	github.com/multiformats/go-multihash v0.0.13
	github.com/nwaples/rardecode v1.0.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.0
	github.com/pierrec/lz4 v2.0.5+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20190826022208-cac0b30c2563
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.7.0
	github.com/shopspring/decimal v0.0.0-20180709203117-cd690d0c9e24
	github.com/stretchr/testify v1.4.0
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	github.com/tendermint/go-amino v0.15.0 // indirect
	github.com/tendermint/iavl v0.0.0-20190701090235-eef65d855b4a
	github.com/tendermint/tm-db v0.4.0
	github.com/whyrusleeping/go-logging v0.0.1
	github.com/willf/bloom v2.0.3+incompatible
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20200115085410-6d4e4cb37c7d
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297
	golang.org/x/sys v0.0.0-20200124204421-9fbb57f87de9
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/urfave/cli.v1 v1.20.0
)

replace github.com/tendermint/iavl => github.com/idena-network/iavl v0.12.3-0.20200120102243-63464c8983b7

go 1.13
