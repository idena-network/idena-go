package protocol

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
	PushFlipCid       = 0x0C
	PullFlip          = 0x0D
	GetForkBlockRange = 0x0E
	FlipKeysPackage    = 0x0F
	FlipKeysPackageCid = 0x10
)

