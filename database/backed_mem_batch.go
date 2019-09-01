package database

import (
	"github.com/tendermint/tm-db"
)

type backedMemBatch struct {
	batch   db.Batch
	touched [][]byte
	touch   func(key []byte)
}

func (b *backedMemBatch) Set(key, value []byte) {
	b.touched = append(b.touched, key)
	b.batch.Set(key, value)
}

func (b *backedMemBatch) Delete(key []byte) {
	b.touched = append(b.touched, key)
	b.batch.Delete(key)
}

func (b *backedMemBatch) Write() {
	b.batch.Write()
	for _, key := range b.touched {
		b.touch(key)
	}
}

func (b *backedMemBatch) WriteSync() {
	b.batch.WriteSync()
	for _, key := range b.touched {
		b.touch(key)
	}
}

func (b *backedMemBatch) Close() {
	b.batch.Close()
}
