module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.5.5
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.22.2
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/cosmos/iavl v0.15.3
	github.com/davecgh/go-spew v1.1.1
	github.com/davidlazar/go-crypto v0.0.0-20190912175916-7055855a373f // indirect
	github.com/deckarep/golang-set v1.7.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/frankban/quicktest v1.11.1 // indirect
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/go-stack/stack v1.8.0
	github.com/golang/protobuf v1.5.2
	github.com/google/tink/go v0.0.0-20200401233402-a389e601043a
	github.com/ipfs/fs-repo-migrations v1.7.1
	github.com/ipfs/go-blockservice v0.1.4
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.5
	github.com/ipfs/go-filestore v0.0.3
	github.com/ipfs/go-ipfs v0.8.0
	github.com/ipfs/go-ipfs-blockstore v0.1.4
	github.com/ipfs/go-ipfs-config v0.12.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipfs-pinner v0.1.1
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-mfs v0.1.2
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipfs/interface-go-ipfs-core v0.4.0
	github.com/klauspost/compress v1.11.12
	github.com/libp2p/go-libp2p-core v0.8.5
	github.com/libp2p/go-msgio v0.0.6
	github.com/libp2p/go-sockaddr v0.1.0 // indirect
	github.com/libp2p/go-yamux v1.4.1
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/multiformats/go-multihash v0.0.15
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.1
	github.com/petermattis/goid v0.0.0-20180202154549-b0b1615b78e5 // indirect
	github.com/pierrec/lz4 v2.5.2+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.7.0
	github.com/shopspring/decimal v0.0.0-20200227202807-02e2044944cc
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	github.com/tendermint/tm-db v0.6.4
	github.com/ulikunitz/xz v0.5.7 // indirect
	github.com/urfave/cli v1.22.5
	github.com/whyrusleeping/go-logging v0.0.1
	github.com/willf/bloom v2.0.3+incompatible
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
	golang.org/x/net v0.0.0-20201021035429-f5854403a974
	golang.org/x/sys v0.0.0-20210309074719-68d13333faf2
	google.golang.org/protobuf v1.26.0
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
)

replace github.com/cosmos/iavl => github.com/idena-network/iavl v0.12.3-0.20210604085842-854e73deab29

replace github.com/libp2p/go-libp2p-pnet => github.com/idena-network/go-libp2p-pnet v0.2.1-0.20200406075059-75d9ee9b85ed

go 1.13
