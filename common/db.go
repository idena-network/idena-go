package common

import db "github.com/tendermint/tm-db"

const (
	MaxMemoryItems = 1000
)

type KeyValuePair struct {
	Key   []byte
	Value []byte
}

func Copy(source, dest db.DB) error {
	var nextStartKey []byte
	moreData := true
	for moreData {
		moreData = false
		it, err := source.Iterator(nextStartKey, nil)
		if err != nil {
			return err
		}
		var data []KeyValuePair
		for ; it.Valid(); it.Next() {
			if len(data) == MaxMemoryItems {
				nextStartKey = it.Key()
				moreData = true
				break
			}
			data = append(data, KeyValuePair{
				Key:   it.Key(),
				Value: it.Value(),
			})
		}
		it.Close()

		for _, item := range data {
			if err := dest.Set(item.Key, item.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func ClearDb(db db.DB) {
	var keys [][]byte
	it, _ := db.Iterator(nil, nil)
	for ; it.Valid(); it.Next() {
		keys = append(keys, it.Key())
	}
	it.Close()
	for _, key := range keys {
		db.Delete(key)
	}
}
