package api

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/keystore"
	"time"
)

// NetApi offers helper utils
type AccountApi struct {
	baseApi *BaseApi
}

func NewAccountApi(baseApi *BaseApi) *AccountApi {
	return &AccountApi{
		baseApi,
	}
}

func (ap *AccountApi) List() []common.Address {
	list := make([]common.Address, 0)
	for _, item := range ap.baseApi.ks.Accounts() {
		list = append(list, item.Address)
	}
	return list
}

func (ap *AccountApi) Create(passPhrase string) (common.Address, error) {
	account, err := ap.baseApi.ks.NewAccount(passPhrase)
	return account.Address, err
}

func (ap *AccountApi) Unlock(addr common.Address, passPhrase string, timeout time.Duration) error {
	return ap.baseApi.ks.TimedUnlock(keystore.Account{Address: addr}, passPhrase, timeout*time.Second)
}

func (ap *AccountApi) Lock(addr common.Address) error {
	return ap.baseApi.ks.Lock(addr)
}
