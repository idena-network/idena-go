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
