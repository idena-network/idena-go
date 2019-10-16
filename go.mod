module github.com/idena-network/idena-go

require (
	github.com/RoaringBitmap/roaring v0.4.18
	github.com/aristanetworks/goarista v0.0.0-20190704150520-f44d68189fd7
	github.com/awnumar/memguard v0.17.1
	github.com/btcsuite/btcd v0.0.0-20190824003749-130ea5bddde3 // indirect
	github.com/cespare/cp v1.1.1
	github.com/coreos/go-semver v0.3.0
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/go-stack/stack v1.8.0
	github.com/gogo/protobuf v1.3.0 // indirect
	github.com/golang/mock v1.3.1 // indirect
	github.com/golang/snappy v0.0.1
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/tink v1.2.2
	github.com/huin/goupnp v1.0.0
	github.com/ipfs/go-blockservice v0.1.0
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-ipfs v0.4.22-0.20190904220133-06153e088e2b
	github.com/ipfs/go-ipfs-config v0.0.11
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-merkledag v0.2.3
	github.com/ipfs/go-mfs v0.1.0
	github.com/ipfs/go-unixfs v0.2.1
	github.com/ipfs/interface-go-ipfs-core v0.2.2
	github.com/jackpal/go-nat-pmp v1.0.1
	github.com/jteeuwen/go-bindata v3.0.7+incompatible
	github.com/mattn/go-isatty v0.0.9 // indirect
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/multiformats/go-multihash v0.0.7
	github.com/nwaples/rardecode v1.0.0 // indirect
	github.com/onsi/ginkgo v1.10.1 // indirect
	github.com/onsi/gomega v1.7.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.0
	github.com/pierrec/lz4 v2.0.5+incompatible // indirect
	github.com/pkg/errors v0.8.1
	github.com/rjeczalik/notify v0.9.2
	github.com/rs/cors v1.6.0
	github.com/shopspring/decimal v0.0.0-20180709203117-cd690d0c9e24
	github.com/stretchr/testify v1.4.0
	github.com/syndtr/goleveldb v1.0.1-0.20190318030020-c3a204f8e965
	github.com/tendermint/go-amino v0.15.0 // indirect
	github.com/tendermint/iavl v0.0.0-20190701090235-eef65d855b4a
	github.com/tendermint/tm-db v0.1.1
	github.com/whyrusleeping/go-logging v0.0.0-20170515211332-0457bb6b88fc
	github.com/willf/bloom v2.0.3+incompatible
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.opencensus.io v0.22.1 // indirect
	golang.org/x/crypto v0.0.0-20190829043050-9756ffdc2472
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297
	golang.org/x/sys v0.0.0-20190904154756-749cb33beabd
	google.golang.org/appengine v1.4.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/urfave/cli.v1 v1.20.0
)

replace github.com/tendermint/iavl => github.com/idena-network/iavl v0.12.3-0.20190919135148-89e4ad773677

replace github.com/libp2p/go-libp2p-kad-dht => github.com/libp2p/go-libp2p-kad-dht v0.2.1-0.20190905000620-96d98235133f
