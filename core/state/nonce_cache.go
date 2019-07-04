package state

import (
	"github.com/idena-network/idena-go/common"
	"sync"
)

type account struct {
	stateObject *stateAccount
	nonce       uint32
}

type NonceCache struct {
	*StateDB

	mu sync.RWMutex

	accounts map[common.Address]map[uint16]*account
}

func NewNonceCache(sdb *StateDB) *NonceCache {
	return &NonceCache{
		StateDB:  sdb.MemoryState(),
		accounts: make(map[common.Address]map[uint16]*account),
	}
}

// GetNonce returns the canonical nonce for the managed or unmanaged account.
// Because GetNonce mutates the DB, we must take a write lock.
func (ns *NonceCache) GetNonce(addr common.Address, epoch uint16) uint32 {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	return ns.getAccount(addr, epoch).nonce
}

// SetNonce sets the new canonical nonce for the managed state
func (ns *NonceCache) SetNonce(addr common.Address, txEpoch uint16, nonce uint32) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	acc := ns.getAccount(addr, txEpoch)
	if acc.nonce < nonce {
		acc.nonce = nonce
	}
}

// populate the managed state
func (ns *NonceCache) getAccount(addr common.Address, epoch uint16) *account {
	if epochs, ok := ns.accounts[addr]; !ok {
		so := ns.GetOrNewAccountObject(addr)
		ns.accounts[addr] = make(map[uint16]*account)
		ns.accounts[addr][epoch] = ns.newAccount(so, epoch)
	} else {
		if acc, ok := epochs[epoch]; !ok {
			so := ns.GetOrNewAccountObject(addr)
			ns.accounts[addr][epoch] = ns.newAccount(so, epoch)
		} else {
			// Always make sure the state account nonce isn't actually higher
			// than the tracked one.
			so := ns.StateDB.getStateAccount(addr)
			if so != nil && acc.nonce < so.Nonce() && so.Epoch() == epoch {
				ns.accounts[addr][epoch] = ns.newAccount(so, epoch)
			}
		}
	}

	return ns.accounts[addr][epoch]
}

func (ns *NonceCache) newAccount(so *stateAccount, epoch uint16) *account {

	nonce := so.Nonce()
	if so.Epoch() < ns.Epoch() || so.Epoch() < epoch {
		nonce = 0
	}

	return &account{so, nonce}
}
