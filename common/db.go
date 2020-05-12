package common

import db "github.com/tendermint/tm-db"

func Copy(source, dest db.DB) error {
	it, err := source.Iterator(nil, nil)
	if err != nil {
		return err
	}
	defer it.Close()

	for ; it.Valid(); it.Next() {
		if err := dest.Set(it.Key(), it.Value()); err != nil {
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
