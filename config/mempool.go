package config

import "time"

type Mempool struct {
	TxPoolQueueSlots      int
	TxPoolExecutableSlots int

	TxPoolAddrQueueLimit      int
	TxPoolAddrExecutableLimit int
	TxLifetime                time.Duration
}

func GetDefaultMempoolConfig() *Mempool {
	return &Mempool{
		TxPoolQueueSlots:      256,
		TxPoolExecutableSlots: 1024,

		TxPoolAddrQueueLimit:      32,
		TxPoolAddrExecutableLimit: 32,
		TxLifetime:                time.Hour * 3,
	}
}
