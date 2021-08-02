package appstate

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/validators"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"sync"
)

type AppState struct {
	ValidatorsCache *validators.ValidatorsCache
	State           *state.StateDB
	NonceCache      *state.NonceCache
	IdentityState   *state.IdentityStateDB
	EvidenceMap     *EvidenceMap

	defaultTree       bool
	prevPrecommitDiff *state.IdentityStateDiff

	readonlyStateCache map[uint64]*AppState
	readonlyStateMutex sync.RWMutex
}

func NewAppState(db dbm.DB, bus eventbus.Bus) (*AppState, error) {
	stateDb, err := state.NewLazy(db)
	if err != nil {
		return nil, err
	}
	identityStateDb, err := state.NewLazyIdentityState(db)
	if err != nil {
		return nil, err
	}
	return &AppState{
		State:         stateDb,
		IdentityState: identityStateDb,
		EvidenceMap:   NewEvidenceMap(bus),
		defaultTree:   true,
	}, nil
}
func (s *AppState) ForCheck(height uint64) (*AppState, error) {
	st, err := s.State.ForCheck(height)
	if err != nil {
		return nil, err
	}
	identityState, err := s.IdentityState.ForCheck(height)
	if err != nil {
		return nil, err
	}

	validatorsCache := s.ValidatorsCache.Clone()
	if validatorsCache.Height() != height {
		validatorsCache = validators.NewValidatorsCache(identityState, st.GodAddress())
		validatorsCache.Load()
	}
	return &AppState{
		State:           st,
		IdentityState:   identityState,
		ValidatorsCache: validatorsCache,
		NonceCache:      s.NonceCache,
	}, nil
}

func (s *AppState) Readonly(height uint64) (*AppState, error) {

	s.readonlyStateMutex.RLock()
	if state, ok := s.readonlyStateCache[height]; ok {
		s.readonlyStateMutex.RUnlock()
		return state, nil
	}
	s.readonlyStateMutex.RUnlock()

	s.readonlyStateMutex.Lock()
	defer s.readonlyStateMutex.Unlock()

	if state, ok := s.readonlyStateCache[height]; ok {
		return state, nil
	}

	st, err := s.State.Readonly(int64(height))
	if err != nil {
		return nil, err
	}
	identityState, err := s.IdentityState.Readonly(height)
	if err != nil {
		return nil, err
	}

	validatorsCache := s.ValidatorsCache.Clone()
	if validatorsCache.Height() != height {
		validatorsCache = validators.NewValidatorsCache(identityState, st.GodAddress())
		validatorsCache.Load()
	}
	state := &AppState{
		State:           st,
		IdentityState:   identityState,
		ValidatorsCache: validatorsCache,
		NonceCache:      s.NonceCache,
	}

	s.readonlyStateCache = map[uint64]*AppState{height: state}
	return state, nil
}

// loads appState
func (s *AppState) ForCheckWithOverwrite(height uint64) (*AppState, error) {

	state, err := s.State.ForCheckWithOverwrite(height)
	if err != nil {
		return nil, err
	}
	identityState, err := s.IdentityState.ForCheckWithOverwrite(height)
	if err != nil {
		return nil, err
	}

	appState := &AppState{
		State:         state,
		IdentityState: identityState,
		NonceCache:    s.NonceCache,
	}
	appState.ValidatorsCache = validators.NewValidatorsCache(appState.IdentityState, appState.State.GodAddress())
	appState.ValidatorsCache.Load()
	return appState, nil
}

func (s *AppState) Initialize(height uint64) error {
	if err := s.State.Load(height); err != nil {
		return err
	}
	if err := s.IdentityState.Load(height); err != nil {
		return err
	}
	s.ValidatorsCache = validators.NewValidatorsCache(s.IdentityState, s.State.GodAddress())
	s.ValidatorsCache.Load()
	cache, err := state.NewNonceCache(s.State)
	if err != nil {
		return err
	}
	s.NonceCache = cache

	return nil
}

func (s *AppState) Precommit() ([]*state.StateTreeDiff, *state.IdentityStateDiff) {
	diff := s.State.Precommit(true)
	s.prevPrecommitDiff = s.IdentityState.Precommit(true)
	return diff, s.prevPrecommitDiff
}

func (s *AppState) FinalizePrecommit(block *types.Block) error {
	_, _, _, err := s.State.Commit(true)
	if err != nil {
		return err
	}
	_, _, _, err = s.IdentityState.Commit(true)
	if err != nil {
		return err
	}

	if block != nil {
		s.ValidatorsCache.RefreshIfUpdated(s.State.GodAddress(), block, s.prevPrecommitDiff)
	}

	return err
}

func (s *AppState) Reset() {
	s.State.Reset()
	s.IdentityState.Reset()
}

func (s *AppState) Commit(block *types.Block) error {
	_, _, _, err := s.State.Commit(true)
	if err != nil {
		return err
	}
	_, _, diff, err := s.IdentityState.Commit(true)
	if err != nil {
		return err
	}

	if block != nil {
		s.ValidatorsCache.RefreshIfUpdated(s.State.GodAddress(), block, diff)
	}

	return err
}

func (s *AppState) CommitAt(height uint64) error {
	_, _, err := s.State.CommitTree(int64(height))
	if err != nil {
		return err
	}

	_, _, err = s.IdentityState.CommitTree(int64(height))

	if err != nil {
		return err
	}

	return err
}

func (s *AppState) ResetTo(height uint64) error {
	err := s.State.ResetTo(height)
	if err != nil {
		return err
	}
	if !s.IdentityState.HasVersion(height) {
		return errors.New("target tree version doesn't exist")
	}
	err = s.IdentityState.ResetTo(height)
	if err != nil {
		return err
	}

	s.ValidatorsCache.Load()
	return nil
}

func (s *AppState) SetPredefinedState(predefinedState *models.ProtoPredefinedState) {
	s.State.SetPredefinedGlobal(predefinedState)
	s.State.SetPredefinedStatusSwitch(predefinedState)
	s.State.SetPredefinedAccounts(predefinedState)
	s.State.SetPredefinedIdentities(predefinedState)
	s.State.SetPredefinedContractValues(predefinedState)
	s.IdentityState.SetPredefinedIdentities(predefinedState)
}

func (s *AppState) CommitTrees(block *types.Block, diff *state.IdentityStateDiff) error {
	_, _, err := s.State.CommitTree(int64(block.Height()))
	if err != nil {
		return err
	}
	_, _, err = s.IdentityState.CommitTree(int64(block.Height()))

	if block != nil {
		s.ValidatorsCache.RefreshIfUpdated(s.State.GodAddress(), block, diff)
	}

	return err
}
