package config

import "time"

type SyncConfig struct {
	FastSync            bool
	ForceFullSync       uint64
	LoadAllFlips        bool
	AllFlipsLoadingTime time.Duration
}
