package validators

import (
	"bytes"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	dbm "github.com/tendermint/tendermint/libs/db"
)

type Validatorsdb struct {
	db  dbm.DB
	log log.Logger
}

func NewValidatorsDb(db dbm.DB) *Validatorsdb {
	return &Validatorsdb{
		db,
		log.New(),
	}
}

func (v *Validatorsdb) LoadValidNodes() ValidNodes {
	data := v.db.Get(validPubKeysKey)
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
	v.db.Set(validPubKeysKey, data)
}
