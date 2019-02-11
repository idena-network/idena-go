package state

import (
	"idena-go/common"
	"sync"
)

type account struct {
	stateObject *stateAccount
	nonce       uint32
}

type NonceCache struct {
	*StateDB

	mu sync.RWMutex

	accounts map[common.Address]*account
}

func NewNonceCache(sdb *StateDB) *NonceCache {
	return &NonceCache{
		StateDB:  sdb.MemoryState(),
		accounts: make(map[common.Address]*account),
	}
}

// GetNonce returns the canonical nonce for the managed or unmanaged account.
// Because GetNonce mutates the DB, we must take a write lock.
func (ns *NonceCache) GetNonce(addr common.Address) uint32 {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if ns.hasAccount(addr) {
		account := ns.getAccount(addr)
		return account.nonce
	} else {
		return ns.StateDB.GetNonce(addr)
	}
}

// SetNonce sets the new canonical nonce for the managed state
func (ns *NonceCache) SetNonce(addr common.Address, nonce uint32) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	so := ns.GetOrNewAccountObject(addr)
	so.SetNonce(nonce)

	ns.accounts[addr] = newAccount(so)
}

func (ns *NonceCache) hasAccount(addr common.Address) bool {
	_, ok := ns.accounts[addr]
	return ok
}

// populate the managed state
func (ns *NonceCache) getAccount(addr common.Address) *account {
	if account, ok := ns.accounts[addr]; !ok {
		so := ns.GetOrNewAccountObject(addr)
		ns.accounts[addr] = newAccount(so)
	} else {
		// Always make sure the state account nonce isn't actually higher
		// than the tracked one.
		so := ns.StateDB.getStateAccount(addr)
		if so != nil && account.nonce < so.Nonce() {
			ns.accounts[addr] = newAccount(so)
		}

	}

	return ns.accounts[addr]
}

func newAccount(so *stateAccount) *account {
	return &account{so, so.Nonce()}
}
