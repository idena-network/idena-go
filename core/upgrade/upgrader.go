package upgrade

import (
	"errors"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/database"
	dbm "github.com/tendermint/tm-db"
	"sync"
	"time"
)

const TargetVersion = config.ConsensusV2

type Upgrader struct {
	config    *config.Config
	appState  *appstate.AppState
	repo      *database.Repo
	votesChan chan *types.Vote
	mutex     sync.RWMutex
	votes     *types.UpgradeVotes
}

func NewUpgrader(config *config.Config, appState *appstate.AppState, db dbm.DB, ) *Upgrader {
	return &Upgrader{
		config:    config,
		appState:  appState,
		repo:      database.NewRepo(db),
		votesChan: make(chan *types.Vote, 10000),
		votes:     types.NewUpgradeVotes(),
	}
}

func (u *Upgrader) Start() {
	u.restore()
	go u.startListening()
}

func (u *Upgrader) ProcessVote(vote *types.Vote) {
	select {
	case u.votesChan <- vote:
	default:
	}
}

func (u *Upgrader) startListening() {
	t := time.NewTicker(time.Minute)
	for {
		select {
		case vote := <-u.votesChan:
			u.processVote(vote)
		case <-t.C:
			u.persist()
		}
	}
}

func (u *Upgrader) Target() config.ConsensusVerson {
	return TargetVersion
}

func (u *Upgrader) UpgradeBits() uint32 {
	if u.IsValidTargetVersion() {
		return uint32(u.Target())
	}
	return 0
}

func (u *Upgrader) IsValidTargetVersion() bool {
	if u.config.Consensus.Version >= u.Target() {
		return false
	}
	now := time.Now().UTC().Unix()
	if now < config.ConsensusVersions[u.Target()].StartActivationDate || now > config.ConsensusVersions[u.Target()].EndActivationDate {
		return false
	}
	return true
}

func (u *Upgrader) ValidateBlock(block *types.Block) error {
	if block.Header.ProposedHeader.Upgrade > 0 {
		if block.Header.ProposedHeader.Upgrade == uint32(u.Target()) && u.CanUpgrade() {
			return nil
		}
		return errors.New("blockchain cannot be upgraded")
	}
	return nil
}

func (u *Upgrader) CanUpgrade() bool {
	if !u.IsValidTargetVersion() {
		return false
	}
	validationDate := u.appState.State.NextValidationTime()
	if validationDate.Sub(time.Now().UTC()) < u.config.Consensus.UpgradeIntervalBeforeValidation {
		return false
	}
	var cnt int
	u.mutex.RLock()
	for voter, upgrade := range u.votes.Dict {
		if u.appState.ValidatorsCache.IsOnlineIdentity(voter) {
			if upgrade == uint32(u.Target()) {
				cnt++
			}
		}
	}
	u.mutex.RUnlock()
	return cnt >= int(0.80*float64(u.appState.ValidatorsCache.OnlineSize())) && cnt >= int(2.0/3.0*float64(u.appState.ValidatorsCache.NetworkSize()))
}

func (u *Upgrader) processVote(vote *types.Vote) {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	if vote.Header.Upgrade > 0 {
		u.votes.Add(vote.VoterAddr(), vote.Header.Upgrade)
	} else {
		u.votes.Remove(vote.VoterAddr())
	}
}

func (u *Upgrader) CompleteMigration() {
	u.config.Consensus = config.ConsensusVersions[u.Target()]
	u.votes = types.NewUpgradeVotes()
}

func (u *Upgrader) persist() {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	u.repo.WriteUpgradeVotes(u.votes)
}

func (u *Upgrader) restore() {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.votes = u.repo.ReadUpgradeVotes()
	if u.votes == nil {
		u.votes = types.NewUpgradeVotes()
	}
}

func (u *Upgrader) UpgradeConfigIfNeeded() {
	genesis := u.repo.ReadIntermediateGenesis()
	if genesis > 0 {
		config.ApplyV2(u.config.Consensus)
	}
}
