package consensus

import (
	"crypto/ecdsa"
	"fmt"
	"idena-go/blockchain"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/config"
	"idena-go/core/state"
	"idena-go/core/validators"
	"idena-go/crypto"
	"idena-go/log"
	"idena-go/pengings"
	"idena-go/protocol"
	"time"
)

type Engine struct {
	chain      *blockchain.Blockchain
	pm         *protocol.ProtocolManager
	log        log.Logger
	state      string
	pubKey     []byte
	config     *config.ConsensusConf
	proposals  *pengings.Proposals
	stateDb    state.Database
	validators *validators.ValidatorsSet
}

func NewEngine(chain *blockchain.Blockchain, pm *protocol.ProtocolManager, proposals *pengings.Proposals, config *config.ConsensusConf,
	stateDb state.Database,
	validators *validators.ValidatorsSet) *Engine {
	return &Engine{
		chain:      chain,
		pm:         pm,
		log:        log.New(),
		config:     config,
		proposals:  proposals,
		stateDb:    stateDb,
		validators: validators,
	}
}

func (engine *Engine) SetPubKey(pubKey *ecdsa.PublicKey) {
	engine.pubKey = crypto.FromECDSAPub(pubKey)
}

func (engine *Engine) Start() {
	go engine.cycle()
}

func (engine *Engine) cycle() {
	for {
		engine.syncBlockchain()

		head := engine.chain.Head
		round := head.Height() + 1
		engine.log.Info(fmt.Sprintf("Start round %v", round))

		engine.state = "Check if I'm proposer"

		isProposer, proposerHash, proposerProof := engine.chain.GetProposerSortition()

		var block *blockchain.Block
		if isProposer {
			engine.state = "Propose block"
			block = engine.proposeBlock(proposerHash, proposerProof)
			engine.log.Info("Selected as proposer", "block", block.Hash().Hex(), "round", round)
		}

		engine.state = "Calculating highest-priority pubkey"

		proposerPubKey := engine.getHighestProposerPubKey(round)

		var proposer string
		if proposer = hexutil.Encode(proposerPubKey); len(proposerPubKey) == 0 {
			proposer = "NOT FOUND"
		}

		engine.log.Info("Selected proposer", "proposer", proposer)

		if proposerPubKey == nil {
			block = engine.chain.GenerateEmptyBlock()
		} else {

			engine.state = "Waiting for block from proposer"
			block = engine.waitForBlock(proposerPubKey)

			if block == nil {
				block = engine.chain.GenerateEmptyBlock()
			}
		}

	}
}

func (engine *Engine) proposeBlock(hash common.Hash, proof []byte) *blockchain.Block {

	block := engine.chain.ProposeBlock(hash, proof)
	engine.pm.ProposeProof(block.Height(), hash, proof, engine.pubKey)
	engine.pm.ProposeBlock(block)

	engine.proposals.AddProposedBlock(block)
	engine.proposals.AddProposeProof(proof, hash, engine.pubKey, block.Height())

	return block
}

func (engine *Engine) getHighestProposerPubKey(round uint64) []byte {
	time.Sleep(engine.config.EstimatedBaVariance + engine.config.WaitSortitionProofDelay)
	return engine.proposals.GetProposerPubKey(round)
}

func (engine *Engine) waitForBlock(proposerPubKey []byte) *blockchain.Block {
	engine.log.Info("Wait for block proposal")
	block, err := engine.proposals.GetProposedBlock(engine.chain.Round(), proposerPubKey, engine.config.WaitBlockDelay)
	if err != nil {
		engine.log.Error("Proposed block is not found", err, err.Error())
		return nil
	}
	return block
}

func (engine *Engine) syncBlockchain() {
	for {
		bestHeight, peerId, err := engine.pm.GetBestHeight()

		head := engine.chain.Head
		if err != nil {
			log.Info("We have peers but don't know their heads. Go to sleep")
			time.Sleep(time.Second * 10)
		} else if bestHeight > head.Height() {
			engine.loadFromPeer(peerId, bestHeight)
		} else {
			engine.log.Info(fmt.Sprintf("Node is synced %v", peerId))
			break
		}
	}
}

func (engine *Engine) loadFromPeer(peerId string, height uint64) {

	engine.log.Info(fmt.Sprintf("Start loading blocks from %v", peerId))
	head := engine.chain.Head
	for h := head.Height() + 1; h <= height; h++ {
		block := engine.pm.GetBlockFromPeer(peerId, h)
		if block == nil {
			engine.log.Warn("Can't load block by height. Try resync")
			time.Sleep(time.Second * 5)
			return
		} else {
			if err := engine.chain.AddBlock(block); err != nil {
				engine.log.Warn(fmt.Sprintf("Block from peer %v is invalid: %v", peerId, err))
				return
			}
		}
	}
}

func (engine *Engine) reduction(block *blockchain.Block) common.Hash {
	engine.state = "Reduction started"
	engine.log.Info("Reduction started", block, block.Hash().Hex())
	return common.Hash{}
}
func (engine *Engine) GetCommitteeVotesTreshold() int {

	var cnt = engine.validators.GetCountOfValidNodes()

	switch cnt {
	case 1:
		return 1
	case 2:
	case 3:
		return 2
	case 4:
	case 5:
		return 3
	case 6:
		return 4
	case 7:
	case 8:
		return 5
	}
	return int(float64(cnt) * engine.config.CommitteePercent * engine.config.ThesholdBa)
}
