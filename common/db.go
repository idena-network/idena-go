package common

import db "github.com/tendermint/tm-db"

type KeyValuePair struct {
	Key   []byte
	Value []byte
}

func Copy(source, dest db.DB) error {
	it, err := source.Iterator(nil, nil)
	if err != nil {
		return err
	}
	var data []KeyValuePair
	for ; it.Valid(); it.Next() {
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
