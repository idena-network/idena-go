package database

import (
	"github.com/tendermint/tm-db"
)

type backedMemBatch struct {
	batch   db.Batch
	touched [][]byte
	touch   func(key []byte)
}

func (b *backedMemBatch) Set(key, value []byte) error {
	b.touched = append(b.touched, key)
	return b.batch.Set(key, value)
}

func (b *backedMemBatch) Delete(key []byte) error {
	b.touched = append(b.touched, key)
	return b.batch.Delete(key)
}

func (b *backedMemBatch) Write() error {
	err := b.batch.Write()
	if err != nil {
		return err
	}
	for _, key := range b.touched {
		b.touch(key)
	}
	return nil
}

func (b *backedMemBatch) WriteSync() error {
	err := b.batch.WriteSync()
	if err != nil {
		return err
	}
	for _, key := range b.touched {
		b.touch(key)
	}
	return nil
}

func (b *backedMemBatch) Close() error {
	return b.batch.Close()
}
