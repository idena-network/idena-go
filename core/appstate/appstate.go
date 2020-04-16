package appstate

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/validators"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
)

type AppState struct {
	ValidatorsCache *validators.ValidatorsCache
	State           *state.StateDB
	NonceCache      *state.NonceCache
	IdentityState   *state.IdentityStateDB
	EvidenceMap     *EvidenceMap
	defaultTree     bool
}

func NewAppState(db dbm.DB, bus eventbus.Bus) *AppState {
	stateDb := state.NewLazy(db)
	identityStateDb := state.NewLazyIdentityState(db)
	return &AppState{
		State:         stateDb,
		IdentityState: identityStateDb,
		EvidenceMap:   NewEvidenceMap(bus),
		defaultTree:   true,
	}
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
	return &AppState{
		State:           st,
		IdentityState:   identityState,
		ValidatorsCache: validatorsCache,
		NonceCache:      s.NonceCache,
	}, nil
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

func (s *AppState) Precommit() *state.IdentityStateDiff {
	s.State.Precommit(true)
	return s.IdentityState.Precommit(true)
}

func (s *AppState) Reset() {
	s.State.Reset()
	s.IdentityState.Reset()
}

func (s *AppState) Commit(block *types.Block) error {
	_, _, err := s.State.Commit(true)
	if err != nil {
		return err
	}
	_, _, _, err = s.IdentityState.Commit(true)

	if block != nil {
		s.ValidatorsCache.RefreshIfUpdated(s.State.GodAddress(), block)
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

func (s *AppState) SetPredefinedState(predefinedState *state.PredefinedState) {
	s.State.SetPredefinedGlobal(predefinedState)
	s.State.SetPredefinedStatusSwitch(predefinedState)
	s.State.SetPredefinedAccounts(predefinedState)
	s.State.SetPredefinedIdentities(predefinedState)
	s.IdentityState.SetPredefinedIdentities(predefinedState)
}

func (s *AppState) UseSyncTree() error {
	if !s.defaultTree {
		return nil
	}
	if err := s.State.SwitchTree(state.SyncTreeKeepEvery, state.SyncTreeKeepRecent); err != nil {
		return err
	}
	if err := s.IdentityState.SwitchTree(state.SyncTreeKeepEvery, state.SyncTreeKeepRecent); err != nil {
		return err
	}
	s.defaultTree = false
	return nil
}

func (s *AppState) UseDefaultTree() error {
	if s.defaultTree {
		return nil
	}
	if err := s.State.FlushToDisk(); err != nil {
		return err
	}

	if err := s.IdentityState.FlushToDisk(); err != nil {
		return err
	}

	if err := s.State.SwitchTree(state.DefaultTreeKeepEvery, state.DefaultTreeKeepRecent); err != nil {
		return err
	}
	if err := s.IdentityState.SwitchTree(state.DefaultTreeKeepEvery, state.DefaultTreeKeepRecent); err != nil {
		return err
	}
	s.defaultTree = true
	return nil
}
