package database

var (
	StateDbPrefix = []byte("st")

	ApprovedIdentityDbPrefix = []byte("aid")

	SnapshotDbPrefix = []byte("snpsht")

	// headBlockKey tracks the latest know full block's hash.
	headBlockKey = []byte("LastBlock")

	headerPrefix = []byte("h")

	headerHashSuffix = []byte("n") // headerPrefix + num (uint64 big endian) + headerHashSuffix -> hash

	finalConsensusPrefix = []byte("f")

	transactionIndexPrefix = []byte("ti")

	certPrefix = []byte("c")

	flipEncryptionPrefix = []byte("key")

	weakCertificatesKey = []byte("weak-cert")

	lastSnapshotKey = []byte("last-snapshot")

	identityStateDiffPrefix = []byte("id-diff")
)
