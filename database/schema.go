package database

var (
	StateDbPrefix = []byte("st")

	ApprovedIdentityDbPrefix = []byte("aid")

	// headBlockKey tracks the latest know full block's hash.
	headBlockKey = []byte("LastBlock")

	headerPrefix = []byte("h")

	headerHashSuffix = []byte("n") // headerPrefix + num (uint64 big endian) + headerHashSuffix -> hash

	finalConsensusPrefix = []byte("f")

	certPrefix = []byte("c")

	flipPrefix = []byte("FLIP")

	flipEncryptionPrefix = []byte("KEYFLIP")
)
