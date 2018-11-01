package blockchain

var (
	// headBlockKey tracks the latest know full block's hash.
	headBlockKey = []byte("LastBlock")

	headerPrefix = []byte("h")

	bodyPrefix = []byte("b")

	headerHashSuffix = []byte("n") // headerPrefix + num (uint64 big endian) + headerHashSuffix -> hash

	finalConsensusPrefix = []byte("f")
)
