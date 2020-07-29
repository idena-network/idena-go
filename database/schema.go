package database

var (
	SnapshotDbPrefix = []byte("snpsht")

	// headBlockKey tracks the latest know full block's hash.
	headBlockKey = []byte("LastBlock")

	headerPrefix = []byte("h")

	headerHashSuffix = []byte("n") // headerPrefix + num (uint64 big endian) + headerHashSuffix -> hash

	finalConsensusPrefix = []byte("f")

	transactionIndexPrefix = []byte("ti")

	receiptIndexPrefix = []byte("ri")

	ownTransactionIndexPrefix = []byte("oti")

	burntCoinsPrefix = []byte("bc")

	certPrefix = []byte("c")

	flipEncryptionPrefix = []byte("key")

	weakCertificatesKey = []byte("weak-cert")

	lastSnapshotKey = []byte("last-snapshot")

	identityStateDiffPrefix = []byte("id-diff")

	preliminaryHeadKey = []byte("preliminary-head")

	activityMonitorKey = []byte("activity")
)
