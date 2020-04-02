module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.4.23
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.22.1
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/davecgh/go-spew v1.1.1
	github.com/davidlazar/go-crypto v0.0.0-20190912175916-7055855a373f // indirect
	github.com/deckarep/golang-set v1.7.1
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/frankban/quicktest v1.9.0 // indirect
	github.com/glycerine/go-unsnap-stream v0.0.0-20190901134440-81cf024a9e0a // indirect
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/go-stack/stack v1.8.0
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/golang/snappy v0.0.1
	github.com/google/tink/go v0.0.0-20200401233402-a389e601043a
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/hashicorp/go-multierror v1.1.0 // indirect
	github.com/ipfs/fs-repo-migrations v1.5.1
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-ds-badger v0.2.3 // indirect
	github.com/ipfs/go-ds-flatfs v0.4.1 // indirect
	github.com/ipfs/go-filestore v1.0.0 // indirect
	github.com/ipfs/go-ipfs v0.4.22-0.20200331193918-bf8db8708551
	github.com/ipfs/go-ipfs-config v0.4.0
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipfs/go-mfs v0.1.1
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipfs/interface-go-ipfs-core v0.2.6
	github.com/jackpal/gateway v1.0.6 // indirect
	github.com/libp2p/go-libp2p-core v0.5.0
	github.com/libp2p/go-msgio v0.0.4
	github.com/libp2p/go-yamux v1.3.4
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/miekg/dns v1.1.29 // indirect
	github.com/multiformats/go-multiaddr v0.2.1
	github.com/multiformats/go-multihash v0.0.13
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.0
	github.com/pierrec/lz4 v2.4.1+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/polydawn/refmt v0.0.0-20190807091052-3d65705ee9f1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.7.0
	github.com/shopspring/decimal v0.0.0-20200227202807-02e2044944cc
	github.com/stretchr/testify v1.5.1
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	github.com/tendermint/go-amino v0.15.1 // indirect
	github.com/tendermint/iavl v0.13.2
	github.com/tendermint/tendermint v0.33.2 // indirect
	github.com/tendermint/tm-db v0.5.1
	github.com/tinylib/msgp v1.1.2 // indirect
	github.com/ulikunitz/xz v0.5.7 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20200321164527-9340289d0ca7 // indirect
	github.com/whyrusleeping/go-logging v0.0.1
	github.com/willf/bloom v2.0.3+incompatible
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.etcd.io/bbolt v1.3.4 // indirect
	go.uber.org/fx v1.11.0 // indirect
	go4.org v0.0.0-20200312051459-7028f7b4a332 // indirect
	golang.org/x/crypto v0.0.0-20200323165209-0ec3e9974c59
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys v0.0.0-20200331124033-c3d80250170d
	golang.org/x/tools v0.0.0-20200401192744-099440627f01 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/urfave/cli.v1 v1.20.0
)

replace github.com/tendermint/iavl => github.com/idena-network/iavl v0.12.3-0.20200120102243-63464c8983b7

go 1.13
