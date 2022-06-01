module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.9.4
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.22.2
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/cosmos/iavl v0.15.3
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.8.0
	github.com/go-stack/stack v1.8.1
	github.com/golang/protobuf v1.5.2
	github.com/google/tink/go v0.0.0-20200401233402-a389e601043a
	github.com/gopherjs/gopherjs v0.0.0-20190910122728-9d188e94fb99 // indirect
	github.com/ipfs/fs-repo-migrations/fs-repo-11-to-12 v1.0.2
	github.com/ipfs/fs-repo-migrations/tools v0.0.0-20211209222258-754a2dcb82ea
	github.com/ipfs/go-blockservice v0.2.1
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-ipfs v0.12.2
	github.com/ipfs/go-ipfs-config v0.18.0
	github.com/ipfs/go-ipfs-files v0.0.9
	github.com/ipfs/go-merkledag v0.5.1
	github.com/ipfs/go-mfs v0.2.1
	github.com/ipfs/go-unixfs v0.3.1
	github.com/ipfs/interface-go-ipfs-core v0.5.2
	github.com/klauspost/compress v1.15.5
	github.com/libp2p/go-libp2p-core v0.11.0
	github.com/libp2p/go-libp2p-pubsub v0.6.0
	github.com/libp2p/go-msgio v0.1.0
	github.com/libp2p/go-yamux v1.4.1
	github.com/mholt/archiver/v3 v3.5.1-0.20210112195346-074da64920d3
	github.com/multiformats/go-multiaddr v0.4.1
	github.com/multiformats/go-multihash v0.1.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.8.2
	github.com/shopspring/decimal v0.0.0-20200227202807-02e2044944cc
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	github.com/tendermint/tm-db v0.6.7
	github.com/urfave/cli v1.22.5
	github.com/whyrusleeping/go-logging v0.0.1
	github.com/willf/bitset v1.1.10 // indirect
	github.com/willf/bloom v2.0.3+incompatible
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/net v0.0.0-20211005001312-d4b1ae081e3b
	golang.org/x/sys v0.0.0-20211025112917-711f33c9992c
	google.golang.org/protobuf v1.27.1
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
)

replace github.com/cosmos/iavl => github.com/idena-network/iavl v0.12.3-0.20211223100228-a33b117aa31e

replace github.com/libp2p/go-libp2p-pnet => github.com/idena-network/go-libp2p-pnet v0.2.1-0.20200406075059-75d9ee9b85ed

replace github.com/ipfs/fs-repo-migrations/fs-repo-11-to-12 => github.com/idena-network/fs-repo-migrations/fs-repo-11-to-12 v0.0.0-20220601101433-9ce72c125fd3

go 1.16
