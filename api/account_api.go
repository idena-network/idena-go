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

func (api *AccountApi) List() []common.Address {
	list := make([]common.Address, 0)
	for _, item := range api.baseApi.ks.Accounts() {
		list = append(list, item.Address)
	}
	return list
}

func (api *AccountApi) Create(passPhrase string) (common.Address, error) {
	account, err := api.baseApi.ks.NewAccount(passPhrase)
	return account.Address, err
}

func (api *AccountApi) Unlock(addr common.Address, passPhrase string, timeout time.Duration) error {
	return api.baseApi.ks.TimedUnlock(keystore.Account{Address: addr}, passPhrase, timeout*time.Second)
}

func (api *AccountApi) Lock(addr common.Address) error {
	return api.baseApi.ks.Lock(addr)
}
