package protocol

import "github.com/coreos/go-semver/semver"

type PeerFeature = string

const Batches = PeerFeature("batches")

const (
	Handshake         = 0x01
	ProposeBlock      = 0x02
	ProposeProof      = 0x03
	Vote              = 0x04
	NewTx             = 0x05
	GetBlockByHash    = 0x06
	GetBlocksRange    = 0x07
	BlocksRange       = 0x08
	FlipBody          = 0x09
	FlipKey           = 0x0A
	SnapshotManifest  = 0x0B
	GetForkBlockRange = 0x0C
	FlipKeysPackage   = 0x0D
	Push              = 0x0E
	Pull              = 0x0F
	Block             = 0x10
	UpdateShardId     = 0x11
	BatchPush         = 0x12
	BatchFlipKey      = 0x13
	Disconnect        = 0x14
)

var batchSupportVersion *semver.Version

func init() {
	batchSupportVersion, _ = semver.NewVersion("1.1.0")
}

func SetSupportedFeatures(peer *protoPeer) {
	if peer.version.Compare(*batchSupportVersion) >= 0 {
		peer.supportedFeatures[Batches] = struct{}{}
	}
}
