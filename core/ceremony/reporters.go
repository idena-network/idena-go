package ceremony

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/state"
)

type reportersToReward struct {
	reportersByFlip         map[int]map[common.Address]*types.Candidate
	reportedFlipsByReporter map[common.Address]map[int]struct{}
	reportersByAddr         map[common.Address]*types.Candidate
}

func newReportersToReward() *reportersToReward {
	return &reportersToReward{
		reportersByFlip:         make(map[int]map[common.Address]*types.Candidate),
		reportedFlipsByReporter: make(map[common.Address]map[int]struct{}),
		reportersByAddr:         make(map[common.Address]*types.Candidate),
	}
}

func (r *reportersToReward) addReport(flipIdx int, reporterAddress common.Address) {
	reporters, ok := r.reportersByFlip[flipIdx]
	if !ok {
		reporters = make(map[common.Address]*types.Candidate)
		r.reportersByFlip[flipIdx] = reporters
	}
	var reporter *types.Candidate
	if curReporter, ok := r.reportersByAddr[reporterAddress]; ok {
		reporter = curReporter
	} else {
		reporter = &types.Candidate{
			Address: reporterAddress,
		}
		r.reportersByAddr[reporterAddress] = reporter
	}
	reporters[reporterAddress] = reporter
	reportedFlips, ok := r.reportedFlipsByReporter[reporterAddress]
	if !ok {
		reportedFlips = make(map[int]struct{})
		r.reportedFlipsByReporter[reporterAddress] = reportedFlips
	}
	reportedFlips[flipIdx] = struct{}{}
}

func (r *reportersToReward) getReportedFlipsCountByReporter(reporter common.Address) int {
	return len(r.reportedFlipsByReporter[reporter])
}

func (r *reportersToReward) getFlipReportsCount(flip int) int {
	return len(r.reportersByFlip[flip])
}

func (r *reportersToReward) deleteFlip(flip int) {
	reporters := r.reportersByFlip[flip]
	delete(r.reportersByFlip, flip)
	for reporter := range reporters {
		delete(r.reportedFlipsByReporter[reporter], flip)
		if len(r.reportedFlipsByReporter[reporter]) == 0 {
			delete(r.reportedFlipsByReporter, reporter)
			delete(r.reportersByAddr, reporter)
		}
	}
}

func (r *reportersToReward) deleteReporter(reporter common.Address) {
	flips := r.reportedFlipsByReporter[reporter]
	delete(r.reportedFlipsByReporter, reporter)
	delete(r.reportersByAddr, reporter)
	for flip := range flips {
		delete(r.reportersByFlip[flip], reporter)
		if len(r.reportersByFlip[flip]) == 0 {
			delete(r.reportersByFlip, flip)
		}
	}
}

func (r *reportersToReward) setValidationResult(address common.Address, newState state.IdentityState, missed bool, flipsByAuthor map[common.Address][]int, cfg *config.ConsensusConf) {
	if !newState.NewbieOrBetter() {
		r.deleteReporter(address)
	} else {
		if reporter, ok := r.reportersByAddr[address]; ok {
			reporter.NewIdentityState = uint8(newState)
		}
	}
	rewardAnyReport := cfg.ReportsRewardPercent > 0
	if missed && !rewardAnyReport {
		for _, flip := range flipsByAuthor[address] {
			r.deleteFlip(flip)
		}
	}
}

func (r *reportersToReward) getReportersByFlipMap() map[int]map[common.Address]*types.Candidate {
	return r.reportersByFlip
}

type grades struct {
	byCandidate map[int]map[int]types.Grade
	byFlip      map[int]*flipGrades
}

type flipGrades struct {
	totalScore, cnt, approveCnt, reportCnt int
}

func newGrades() *grades {
	return &grades{
		byFlip:      make(map[int]*flipGrades),
		byCandidate: make(map[int]map[int]types.Grade),
	}
}

func (g *grades) addGrade(candidateIdx, flipIdx int, grade types.Grade) {
	flips, ok := g.byCandidate[candidateIdx]
	if !ok {
		flips = make(map[int]types.Grade)
		g.byCandidate[candidateIdx] = flips
	}
	flips[flipIdx] = grade

	flip, ok := g.byFlip[flipIdx]
	if !ok {
		flip = new(flipGrades)
		g.byFlip[flipIdx] = flip
	}
	flip.cnt++

	if grade >= types.GradeD {
		flip.approveCnt++
	}

	flip.totalScore += grade.Score()

	if grade == types.GradeReported {
		flip.reportCnt++
	}
}

func (g *grades) deleteGrades(candidateIdx int) {
	for flipIdx, grade := range g.byCandidate[candidateIdx] {
		flip := g.byFlip[flipIdx]
		flip.cnt--
		if grade >= types.GradeD {
			flip.approveCnt--
		}
		flip.totalScore -= grade.Score()
		if grade == types.GradeReported {
			flip.reportCnt--
		}
	}
	delete(g.byCandidate, candidateIdx)
}

func (g *grades) flip(flipIdx int) (totalGradeScore, gradeCnt, approveCnt, reportCnt int) {
	flipGrades, ok := g.byFlip[flipIdx]
	if !ok {
		return
	}
	return flipGrades.totalScore, flipGrades.cnt, flipGrades.approveCnt, flipGrades.reportCnt
}
