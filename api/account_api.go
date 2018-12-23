package api

import (
	"idena-go/common"
	"idena-go/keystore"
	"time"
)

// NetApi offers helper utils
type AccountApi struct {
	ks *keystore.KeyStore
}

func NewAccountApi(ks *keystore.KeyStore) *AccountApi {
	return &AccountApi{
		ks,
	}
}

func (ap *AccountApi) List() []common.Address {
	list := make([]common.Address, 0)
	for _, item := range ap.ks.Accounts() {
		list = append(list, item.Address)
	}
	return list
}

func (ap *AccountApi) Create(passPhrase string) (common.Address, error) {
	account, err := ap.ks.NewAccount(passPhrase)
	return account.Address, err
}

func (ap *AccountApi) Unlock(addr common.Address, passPhrase string, timeout time.Duration) error {
	return ap.ks.TimedUnlock(keystore.Account{Address: addr}, passPhrase, timeout*time.Second)
}

func (ap *AccountApi) Lock(addr common.Address) error {
	return ap.ks.Lock(addr)
}
