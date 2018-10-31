package validators

import (
	"bytes"
	"idena-go/idenadb"
	"idena-go/log"
	"idena-go/rlp"
)

type Validatorsdb struct {
	db  idenadb.Database
	log log.Logger
}

func NewValidatorsDb(db idenadb.Database) *Validatorsdb {
	return &Validatorsdb{
		db,
		log.New(),
	}
}

func (v *Validatorsdb) LoadValidNodes() (ValidNodes) {
	data, _ := v.db.Get(validPubKeysKey)
	if len(data) == 0 {
		return ValidNodes{}
	}
	validNodes := new(ValidNodes)
	if err := rlp.Decode(bytes.NewReader(data), validNodes); err != nil {
		log.Error("Invalid valid nodes RLP", "err", err)
		return nil
	}
	return *validNodes
}
func (v *Validatorsdb) WriteValidNodes(nodes ValidNodes) {

	data, err := rlp.EncodeToBytes(nodes)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	if err := v.db.Put(validPubKeysKey, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}
