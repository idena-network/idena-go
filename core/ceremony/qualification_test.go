package ceremony

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tm-db"
	"math/rand"
	"testing"
	"time"
)

func Test_getAnswersCount(t *testing.T) {
	require := require.New(t)

	ans := fillArray(10, 13, 3)

	resLeft, resRight, none := getAnswersCount(ans)

	require.Equal(uint(10), resLeft)
	require.Equal(uint(13), resRight)
	require.Equal(uint(3), none)
}

func Test_qualifyOneFlip(t *testing.T) {
	require := require.New(t)

	ans := fillArray(6, 1, 1)
	q := qualifyOneFlip(ans, 4, 18, 4)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeA, q.grade)

	ans = fillArray(6, 0, 0)
	q = qualifyOneFlip(ans, 2, 6, 2)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeC, q.grade)

	ans = fillArray(7, 0, 0)
	q = qualifyOneFlip(ans, 2, 8, 2)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(7, 0, 0)
	q = qualifyOneFlip(ans, 3, 12, 3)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeB, q.grade)

	ans = fillArray(6, 1, 1)
	q = qualifyOneFlip(ans, 4, 13, 3)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeB, q.grade)

	ans = fillArray(6, 1, 1)
	q = qualifyOneFlip(ans, 4, 12, 4)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeC, q.grade)

	ans = fillArray(6, 1, 1)
	q = qualifyOneFlip(ans, 4, 8, 4)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(6, 1, 1)
	q = qualifyOneFlip(ans, 5, 6, 3)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeReported, q.grade)

	ans = fillArray(25, 75, 0)
	q = qualifyOneFlip(ans, 0, 0, 0)
	require.Equal(Qualified, q.status)
	require.Equal(types.Right, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(0, 10, 0)
	q = qualifyOneFlip(ans, 0, 0, 0)
	require.Equal(Qualified, q.status)
	require.Equal(types.Right, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(15, 3, 4)
	q = qualifyOneFlip(ans, 0, 0, 0)
	require.Equal(WeaklyQualified, q.status)
	require.Equal(types.Left, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(30, 66, 4)
	q = qualifyOneFlip(ans, 0, 0, 0)
	require.Equal(WeaklyQualified, q.status)
	require.Equal(types.Right, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(4, 4, 4)
	q = qualifyOneFlip(ans, 0, 0, 0)
	require.Equal(NotQualified, q.status)
	require.Equal(types.None, q.answer)
	require.Equal(types.GradeD, q.grade)

	ans = fillArray(1, 2, 10)
	q = qualifyOneFlip(ans, 0, 0, 0)
	require.Equal(QualifiedByNone, q.status)
	require.Equal(types.None, q.answer)
	require.Equal(types.GradeD, q.grade)
}

func Test_getFlipStatusForCandidate(t *testing.T) {
	shortAnswers := types.NewAnswers(3)
	shortAnswers.Right(0)
	shortAnswers.Left(2)

	notApprovedFlips := mapset.NewSet()
	notApprovedFlips.Add(5)
	notApprovedFlips.Add(7)

	r := require.New(t)
	r.Equal(NotQualified, getFlipStatusForCandidate(6, 0, NotQualified, notApprovedFlips, shortAnswers, true))
	r.Equal(Qualified, getFlipStatusForCandidate(6, 0, Qualified, notApprovedFlips, shortAnswers, true))

	r.Equal(NotQualified, getFlipStatusForCandidate(5, 0, NotQualified, notApprovedFlips, shortAnswers, true))
	r.Equal(NotQualified, getFlipStatusForCandidate(5, 1, NotQualified, notApprovedFlips, shortAnswers, true))
	r.Equal(NotQualified, getFlipStatusForCandidate(5, 2, NotQualified, notApprovedFlips, shortAnswers, true))

	r.Equal(Qualified, getFlipStatusForCandidate(5, 0, Qualified, notApprovedFlips, shortAnswers, true))
	r.Equal(NotQualified, getFlipStatusForCandidate(5, 1, Qualified, notApprovedFlips, shortAnswers, true))
	r.Equal(Qualified, getFlipStatusForCandidate(5, 2, Qualified, notApprovedFlips, shortAnswers, true))

	r.Equal(Qualified, getFlipStatusForCandidate(5, 1, Qualified, notApprovedFlips, shortAnswers, false))
}

func Test_qualifyCandidate(t *testing.T) {
	// given
	db := db2.NewMemDB()
	epochDb := database.NewEpochDb(db, 1)

	shortFlipsToSolve := []int{10, 11, 12, 13, 20, 21, 22}
	longFlipsToSolve := []int{10, 11, 12, 13, 20, 21, 22, 23, 24}

	flipQualificationMap := make(map[int]FlipQualification)
	flipQualificationMap[10] = FlipQualification{
		status: Qualified,
		answer: types.Left,
		grade:  types.GradeReported,
	}
	flipQualificationMap[11] = FlipQualification{
		status: NotQualified,
		answer: types.None,
		grade:  types.GradeD,
	}
	flipQualificationMap[12] = FlipQualification{
		status: NotQualified,
		answer: types.None,
		grade:  types.GradeD,
	}
	flipQualificationMap[13] = FlipQualification{
		status: NotQualified,
		answer: types.None,
		grade:  types.GradeD,
	}
	flipQualificationMap[20] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Right,
		grade:  types.GradeD,
	}
	flipQualificationMap[21] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Right,
		grade:  types.GradeD,
	}
	flipQualificationMap[22] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Left,
		grade:  types.GradeD,
	}
	flipQualificationMap[23] = FlipQualification{
		status: Qualified,
		answer: types.Right,
		grade:  types.GradeD,
	}
	flipQualificationMap[24] = FlipQualification{
		status: Qualified,
		answer: types.Right,
		grade:  types.GradeD,
	}

	notApprovedFlips := mapset.NewSet()
	notApprovedFlips.Add(11)
	notApprovedFlips.Add(12)

	candidate := tests.GetRandAddr()
	maliciousCandidate := tests.GetRandAddr()

	short := types.NewAnswers(uint(len(shortFlipsToSolve)))
	short.Left(0)
	short.Right(3)
	short.Right(4)
	short.Left(5)
	short.Left(6)

	long := types.NewAnswers(uint(len(longFlipsToSolve)))
	long.Left(0)
	long.Right(3)
	long.Right(4)
	long.Left(5)
	long.Left(6)
	long.Left(7)
	long.Right(8)

	shortAnswer := short.Bytes()
	longAnswer := long.Bytes()
	salt := []byte{0x1, 0x10, 0x25}
	epochDb.WriteAnswerHash(candidate, crypto.Hash(append(shortAnswer, salt...)), time.Now())
	epochDb.WriteAnswerHash(maliciousCandidate, crypto.Hash(append(shortAnswer, salt...)), time.Now())

	k, _ := p256.GenerateKey()
	h, proof := k.Evaluate([]byte("aabbcc"))

	key, _ := crypto.GenerateKey()
	attachment := attachments.CreateShortAnswerAttachment(shortAnswer, getWordsRnd(h))
	maliciousAttachment := attachments.CreateShortAnswerAttachment(shortAnswer, getWordsRnd(h))

	longAttachment := attachments.CreateLongAnswerAttachment(longAnswer, proof, salt, ecies.ImportECDSA(key))
	maliciousLongAttachment := attachments.CreateLongAnswerAttachment(longAnswer, proof, []byte{0x6}, ecies.ImportECDSA(key))

	q := qualification{
		shortAnswers: map[common.Address][]byte{
			candidate:          attachment,
			maliciousCandidate: maliciousAttachment,
		},
		longAnswers: map[common.Address][]byte{
			candidate:          longAttachment,
			maliciousCandidate: maliciousLongAttachment,
		},
		epochDb: epochDb,
	}

	// when
	shortPoint, shortQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(candidate, flipQualificationMap, shortFlipsToSolve, true, notApprovedFlips)
	longPoint, longQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(candidate, flipQualificationMap, longFlipsToSolve, false, notApprovedFlips)

	mLongPoint, mLongQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(maliciousCandidate, flipQualificationMap, longFlipsToSolve, false, notApprovedFlips)

	// then
	require.Equal(t, float32(2.5), shortPoint)
	require.Equal(t, uint32(3), shortQualifiedFlipsCount)
	require.Equal(t, float32(4.5), longPoint)
	require.Equal(t, uint32(6), longQualifiedFlipsCount)

	require.Equal(t, float32(0), mLongPoint)
	require.Equal(t, uint32(9), mLongQualifiedFlipsCount)
}

func Test_qualifyCandidateWithFewFlips(t *testing.T) {
	// given
	db := db2.NewMemDB()
	epochDb := database.NewEpochDb(db, 1)
	shortFlipsToSolve := []int{22, 55}
	candidate := tests.GetRandAddr()

	flipQualificationMap := make(map[int]FlipQualification)
	flipQualificationMap[22] = FlipQualification{
		status: Qualified,
		answer: types.Left,
		grade:  types.GradeD,
	}
	flipQualificationMap[55] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Left,
		grade:  types.GradeD,
	}

	short := types.NewAnswers(uint(len(shortFlipsToSolve)))
	short.Left(0)
	short.Right(1)

	shortAnswer := short.Bytes()
	salt := []byte{0x1, 0x10, 0x25, 0x28}
	epochDb.WriteAnswerHash(candidate, crypto.Hash(append(shortAnswer, salt...)), time.Now())
	attachment := attachments.CreateShortAnswerAttachment(shortAnswer, 100)
	q := qualification{
		shortAnswers: map[common.Address][]byte{
			candidate: attachment,
		},
		epochDb: epochDb,
	}
	shortPoint, shortQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(candidate, flipQualificationMap, shortFlipsToSolve, true, mapset.NewSet())

	require.Equal(t, float32(1.5), shortPoint)
	require.Equal(t, uint32(2), shortQualifiedFlipsCount)
}

func fillArray(left int, right int, none int) []types.Answer {
	var ans []types.Answer

	for i := 0; i < left; i++ {
		ans = append(ans, types.Left)
	}
	for i := 0; i < right; i++ {
		ans = append(ans, types.Right)
	}
	for i := 0; i < none; i++ {
		ans = append(ans, types.None)
	}

	rand.Shuffle(len(ans), func(i, j int) {
		ans[i], ans[j] = ans[j], ans[i]
	})

	return ans
}

func TestQualification_addAnswers(t *testing.T) {
	q := qualification{
		shortAnswers: map[common.Address][]byte{},
		longAnswers:  map[common.Address][]byte{},
	}
	sender := common.Address{0x1}
	q.addAnswers(true, sender, []byte{0x1})
	q.addAnswers(true, sender, []byte{0x2})
	q.addAnswers(false, sender, []byte{0x3})
	q.addAnswers(false, sender, []byte{0x4})

	sender2 := common.Address{0x2}
	q.addAnswers(true, sender2, []byte{0x1})

	require.Equal(t, []byte{0x1}, q.shortAnswers[sender])
	require.Equal(t, []byte{0x3}, q.longAnswers[sender])
	require.Equal(t, []byte{0x1}, q.shortAnswers[sender2])
}

func TestQualification_qualifyFlips(t *testing.T) {
	addrWithIgnoredReports := tests.GetRandAddr()
	addrWithoutReports := tests.GetRandAddr()
	addr1 := tests.GetRandAddr()
	addr2 := tests.GetRandAddr()
	addr3 := tests.GetRandAddr()
	candidates := []*candidate{
		{Address: addrWithIgnoredReports},
		{Address: addrWithoutReports},
		{Address: addr1},
		{Address: addr2},
		{Address: addr3},
	}
	reportedFlipIdx := 5
	flipsPerCandidate := [][]int{
		{reportedFlipIdx, 6, 7},
		{reportedFlipIdx, 6, 7},
		{reportedFlipIdx, 6, 7, 8},
		{reportedFlipIdx, 6, 7, 8},
		{reportedFlipIdx, 6, 7},
	}

	key, _ := crypto.GenerateKey()

	long0 := types.NewAnswers(uint(len(flipsPerCandidate[0])))
	long0.Left(0)
	long0.Grade(0, types.GradeReported)
	long0.Left(1)
	long0.Grade(1, types.GradeD)
	long0.Right(2)
	long0.Grade(2, types.GradeReported)
	longAttachment0 := attachments.CreateLongAnswerAttachment(long0.Bytes(), nil, nil, ecies.ImportECDSA(key))

	long1 := types.NewAnswers(uint(len(flipsPerCandidate[1])))
	long1.Left(0)
	long1.Grade(0, types.GradeA)
	long1.Left(1)
	long1.Grade(1, types.GradeD)
	long1.Right(2)
	long1.Grade(2, types.GradeC)
	longAttachment1 := attachments.CreateLongAnswerAttachment(long1.Bytes(), nil, nil, ecies.ImportECDSA(key))

	long2 := types.NewAnswers(uint(len(flipsPerCandidate[2])))
	long2.Left(0)
	long2.Grade(0, types.GradeReported)
	long2.Left(1)
	long2.Grade(1, types.GradeA)
	long2.Right(2)
	long2.Grade(2, types.GradeReported)
	long2.Right(3)
	long2.Grade(3, types.GradeA)
	longAttachment2 := attachments.CreateLongAnswerAttachment(long2.Bytes(), nil, nil, ecies.ImportECDSA(key))

	long3 := types.NewAnswers(uint(len(flipsPerCandidate[3])))
	long3.Left(0)
	long3.Grade(0, types.GradeReported)
	long3.Left(1)
	long3.Grade(1, types.GradeA)
	long3.Right(2)
	long3.Grade(2, types.GradeReported)
	long3.Right(3)
	long3.Grade(3, types.GradeA)
	longAttachment3 := attachments.CreateLongAnswerAttachment(long3.Bytes(), nil, nil, ecies.ImportECDSA(key))

	long4 := types.NewAnswers(uint(len(flipsPerCandidate[4])))
	long4.Left(0)
	long4.Grade(0, types.GradeReported)
	long4.Left(1)
	long4.Grade(1, types.GradeA)
	long4.Right(2)
	long4.Grade(2, types.GradeB)
	longAttachment4 := attachments.CreateLongAnswerAttachment(long4.Bytes(), nil, nil, ecies.ImportECDSA(key))

	q := qualification{
		longAnswers: map[common.Address][]byte{
			addrWithIgnoredReports: longAttachment0,
			addrWithoutReports:     longAttachment1,
			addr1:                  longAttachment2,
			addr2:                  longAttachment3,
			addr3:                  longAttachment4,
		},
	}

	flipQualifications, reportersToReward := q.qualifyFlips(9, candidates, flipsPerCandidate)

	require.Equal(t, 9, len(flipQualifications))
	require.NotNil(t, reportersToReward)

	require.Equal(t, types.GradeReported, flipQualifications[reportedFlipIdx].grade)
	require.Equal(t, types.Left, flipQualifications[reportedFlipIdx].answer)
	require.Equal(t, Qualified, flipQualifications[reportedFlipIdx].status)

	require.Equal(t, types.GradeB, flipQualifications[6].grade)
	require.Equal(t, types.Left, flipQualifications[6].answer)
	require.Equal(t, Qualified, flipQualifications[6].status)

	require.Equal(t, types.GradeB, flipQualifications[7].grade)
	require.Equal(t, types.Right, flipQualifications[7].answer)
	require.Equal(t, Qualified, flipQualifications[7].status)

	require.Equal(t, types.GradeA, flipQualifications[8].grade)
	require.Equal(t, types.Right, flipQualifications[8].answer)
	require.Equal(t, Qualified, flipQualifications[8].status)

	require.Equal(t, 3, len(reportersToReward.reportersByAddr))

	require.Equal(t, 3, len(reportersToReward.reportedFlipsByReporter))
	require.Equal(t, 1, len(reportersToReward.reportedFlipsByReporter[addr1]))
	_, ok := reportersToReward.reportedFlipsByReporter[addr1][reportedFlipIdx]
	require.True(t, ok)
	require.Equal(t, 1, len(reportersToReward.reportedFlipsByReporter[addr2]))
	_, ok = reportersToReward.reportedFlipsByReporter[addr2][reportedFlipIdx]
	require.True(t, ok)
	require.Equal(t, 1, len(reportersToReward.reportedFlipsByReporter[addr2]))
	_, ok = reportersToReward.reportedFlipsByReporter[addr3][reportedFlipIdx]
	require.True(t, ok)

	require.Equal(t, 1, len(reportersToReward.reportersByFlip))
	require.Equal(t, 3, len(reportersToReward.reportersByFlip[reportedFlipIdx]))
	require.NotNil(t, reportersToReward.reportersByFlip[reportedFlipIdx][addr1])
	require.NotNil(t, reportersToReward.reportersByFlip[reportedFlipIdx][addr2])
	require.NotNil(t, reportersToReward.reportersByFlip[reportedFlipIdx][addr3])
}
