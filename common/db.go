package common

import db "github.com/tendermint/tm-db"

const (
	MaxMemoryItems = 1000
)

type keyValuePair struct {
	key   []byte
	value []byte
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
		var data []keyValuePair
		for ; it.Valid(); it.Next() {
			if len(data) == MaxMemoryItems {
				nextStartKey = it.Key()
				moreData = true
				break
			}
			data = append(data, keyValuePair{
				key:   it.Key(),
				value: it.Value(),
			})
		}
		it.Close()

		for _, item := range data {
			if err := dest.Set(item.key, item.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func ClearDb(db db.DB) error {
	moreData := true
	for moreData {
		moreData = false
		it, err := db.Iterator(nil, nil)
		if err != nil {
			return err
		}
		var keys [][]byte
		for ; it.Valid(); it.Next() {
			if len(keys) == MaxMemoryItems {
				moreData = true
				break
			}
			keys = append(keys, it.Key())
		}
		it.Close()

		for _, key := range keys {
			if err := db.Delete(key); err != nil {
				return err
			}
		}
	}
	return nil
}
