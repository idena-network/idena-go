package appstate

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/validators"
	dbm "github.com/tendermint/tm-cmn/db"
	"github.com/pkg/errors"
)

type AppState struct {
	ValidatorsCache *validators.ValidatorsCache
	State           *state.StateDB
	NonceCache      *state.NonceCache
	IdentityState   *state.IdentityStateDB
	EvidenceMap     *EvidenceMap
}

func NewAppState(db dbm.DB, bus eventbus.Bus) *AppState {
	stateDb := state.NewLazy(db)
	identityStateDb := state.NewLazyIdentityState(db)
	return &AppState{
		State:         stateDb,
		IdentityState: identityStateDb,
		EvidenceMap:   NewEvidenceMap(bus),
	}
}

func (s *AppState) Readonly(height uint64) *AppState {
	state, _ := s.State.Readonly(height)
	identityState, _ := s.IdentityState.Readonly(height)
	return &AppState{
		State:           state,
		IdentityState:   identityState,
		ValidatorsCache: s.ValidatorsCache,
		NonceCache:      s.NonceCache,
	}
}

func (s *AppState) ForCheckWithNewCache(height uint64) (*AppState, error) {

	state, err := s.State.ForCheck(height)
	if err != nil {
		return nil, err
	}
	identityState, err := s.IdentityState.ForCheck(height)
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

func (s *AppState) Initialize(height uint64) {
	s.State.Load(height)
	s.IdentityState.Load(height)
	s.ValidatorsCache = validators.NewValidatorsCache(s.IdentityState, s.State.GodAddress())
	s.ValidatorsCache.Load()
	s.NonceCache = state.NewNonceCache(s.State)
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
		s.ValidatorsCache.RefreshIfUpdated(block)
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
	return err
}
