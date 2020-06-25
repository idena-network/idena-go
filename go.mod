module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.4.23
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.22.2
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/go-stack/stack v1.8.0
	github.com/golang/protobuf v1.4.2
	github.com/google/tink/go v0.0.0-20200401233402-a389e601043a
	github.com/ipfs/fs-repo-migrations v1.6.0
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.0.6

	github.com/ipfs/go-ipfs v0.6.0
	github.com/ipfs/go-ipfs-config v0.8.0
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-mfs v0.1.2
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipfs/interface-go-ipfs-core v0.3.0
	github.com/klauspost/compress v1.10.7
	github.com/libp2p/go-libp2p-core v0.5.7
	github.com/libp2p/go-msgio v0.0.4
	github.com/libp2p/go-yamux v1.3.7
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/mmcloughlin/avo v0.0.0-20200523190732-4439b6b2c061 // indirect
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multihash v0.0.13
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.7.0
	github.com/shopspring/decimal v0.0.0-20200227202807-02e2044944cc
	github.com/stretchr/testify v1.6.1
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	github.com/tendermint/iavl v0.13.2
	github.com/tendermint/tm-db v0.6.0
	github.com/ulikunitz/xz v0.5.7 // indirect
	github.com/urfave/cli v1.22.4
	github.com/whyrusleeping/go-logging v0.0.1
	github.com/willf/bloom v2.0.3+incompatible
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37
	golang.org/x/net v0.0.0-20200520182314-0ba52f642ac2
	golang.org/x/sys v0.0.0-20200523222454-059865788121
	google.golang.org/protobuf v1.23.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
)

replace github.com/tendermint/iavl => github.com/idena-network/iavl v0.12.3-0.20200512113010-084227856fe0

replace github.com/libp2p/go-libp2p-pnet => github.com/idena-network/go-libp2p-pnet v0.2.1-0.20200406075059-75d9ee9b85ed

go 1.13
