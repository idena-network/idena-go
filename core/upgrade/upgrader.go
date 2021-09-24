package upgrade

import (
	"errors"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	dbm "github.com/tendermint/tm-db"
	"sync"
	"time"
)

const TargetVersion = config.ConsensusV6

type Upgrader struct {
	config           *config.Config
	appState         *appstate.AppState
	repo             *database.Repo
	votesChan        chan *types.Vote
	mutex            sync.RWMutex
	votes            *types.UpgradeVotes
	throttlingLogger log.ThrottlingLogger
}

func NewUpgrader(config *config.Config, appState *appstate.AppState, db dbm.DB) *Upgrader {
	throttlingLogger := log.NewThrottlingLogger(log.New("component", "upgrader"))
	return &Upgrader{
		config:           config,
		appState:         appState,
		repo:             database.NewRepo(db),
		votesChan:        make(chan *types.Vote, 10000),
		votes:            types.NewUpgradeVotes(),
		throttlingLogger: throttlingLogger,
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
		u.throttlingLogger.Warn("Vote skipped")
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
	if u.config.Consensus.Version < TargetVersion {
		return u.config.Consensus.Version + 1
	}
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
	return cnt >= int(0.80*float64(u.appState.ValidatorsCache.OnlineSize())) && cnt >= int(2.0/3.0*float64(u.appState.ValidatorsCache.ForkCommitteeSize()))
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

func (u *Upgrader) persist() {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	u.repo.WriteUpgradeVotes(u.votes)

	cnt:=0
	for voter, upgrade := range u.votes.Dict {
		if u.appState.ValidatorsCache.IsOnlineIdentity(voter) {
			if upgrade == uint32(u.Target()) {
				cnt++
			}
		}
	}
	log.Info("actual votes", "cnt", cnt)
}

func (u *Upgrader) restore() {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.votes = u.repo.ReadUpgradeVotes()
	if u.votes == nil {
		u.votes = types.NewUpgradeVotes()
	} else {
		log.Info("Restore upgrade votes", "cnt", len(u.votes.Dict))
	}
}

func (u *Upgrader) UpgradeConfigTo(ver uint32) (prev *config.ConsensusConf) {
	if ver > 0 && ver > uint32(u.config.Consensus.Version) {
		prevVersion := *u.config.Consensus
		for v := u.config.Consensus.Version + 1; v <= config.ConsensusVerson(ver); v++ {
			config.ApplyConsensusVersion(v, u.config.Consensus)
		}
		log.Info("Consensus config transformed to", "ver", ver)
		return &prevVersion
	}
	log.Info("Consensus config didn't transformed", "current version", u.config.Consensus.Version, "target", ver)
	return nil
}

func (u *Upgrader) RevertConfig(prevConfig *config.ConsensusConf) {
	u.config.Consensus = prevConfig
	log.Info("Consensus config reverted to", "ver", prevConfig.Version)
}

func (u *Upgrader) CompleteMigration() {
	u.UpgradeConfigTo(uint32(u.Target()))
	u.votes = types.NewUpgradeVotes()
}

// use to migrate identity state db in fast sync mode
func (u *Upgrader) MigrateIdentityStateDb() {
	// no migration for v2
}

func (u *Upgrader) GetVotes() map[common.Address]uint32 {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	res := make(map[common.Address]uint32, len(u.votes.Dict))
	for voter, upgrade := range u.votes.Dict {
		if u.appState.ValidatorsCache.IsOnlineIdentity(voter) {
			res[voter] = upgrade
		}
	}
	return res
}
