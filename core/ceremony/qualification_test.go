package ceremony

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tm-cmn/db"
	"math/rand"
	"testing"
	"time"
)

func Test_getAnswersCount(t *testing.T) {
	require := require.New(t)

	ans := fillArray(10, 13, 4, 3)

	resLeft, resRight, resInapp := getAnswersCount(ans)

	require.Equal(uint(10), resLeft)
	require.Equal(uint(13), resRight)
	require.Equal(uint(4), resInapp)
}

func Test_qualifyOneFlip(t *testing.T) {
	require := require.New(t)

	ans := fillArray(6, 1, 1, 0)
	q := qualifyOneFlip(ans)
	require.Equal(Qualified, q.status)
	require.Equal(types.Left, q.answer)

	ans = fillArray(25, 0, 75, 0)
	q = qualifyOneFlip(ans)
	require.Equal(Qualified, q.status)
	require.Equal(types.Inappropriate, q.answer)

	ans = fillArray(0, 10, 0, 0)
	q = qualifyOneFlip(ans)
	require.Equal(Qualified, q.status)
	require.Equal(types.Right, q.answer)

	ans = fillArray(15, 3, 0, 4)
	q = qualifyOneFlip(ans)
	require.Equal(WeaklyQualified, q.status)
	require.Equal(types.Left, q.answer)

	ans = fillArray(30, 66, 0, 4)
	q = qualifyOneFlip(ans)
	require.Equal(WeaklyQualified, q.status)
	require.Equal(types.Right, q.answer)

	ans = fillArray(4, 4, 8, 0)
	q = qualifyOneFlip(ans)
	require.Equal(WeaklyQualified, q.status)
	require.Equal(types.Inappropriate, q.answer)

	ans = fillArray(4, 4, 4, 0)
	q = qualifyOneFlip(ans)
	require.Equal(NotQualified, q.status)
	require.Equal(types.None, q.answer)
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

	flipsToSolve := []int{10, 11, 12, 13, 20, 21, 22, 23, 24}

	flipQualificationMap := make(map[int]FlipQualification)
	flipQualificationMap[10] = FlipQualification{
		status: Qualified,
		answer: types.Left,
	}
	flipQualificationMap[11] = FlipQualification{
		status: Qualified,
		answer: types.Right,
	}
	flipQualificationMap[12] = FlipQualification{
		status: Qualified,
		answer: types.Right,
	}
	flipQualificationMap[13] = FlipQualification{
		status: NotQualified,
		answer: types.Right,
	}
	flipQualificationMap[20] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Right,
	}
	flipQualificationMap[21] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Right,
	}
	flipQualificationMap[22] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Inappropriate,
	}
	flipQualificationMap[23] = FlipQualification{
		status: Qualified,
		answer: types.Right,
	}
	flipQualificationMap[24] = FlipQualification{
		status: Qualified,
		answer: types.Right,
	}

	notApprovedFlips := mapset.NewSet()
	notApprovedFlips.Add(23)
	notApprovedFlips.Add(24)

	candidate := tests.GetRandAddr()
	maliciousCandidate := tests.GetRandAddr()

	shortAnswer := []byte{57, 99}
	epochDb.WriteAnswerHash(candidate, rlp.Hash(shortAnswer), time.Now())
	epochDb.WriteAnswerHash(maliciousCandidate, rlp.Hash([]byte{0x0}), time.Now())

	q := qualification{
		shortAnswers: map[common.Address][]byte{
			candidate:          shortAnswer, // 11100101100011
			maliciousCandidate: shortAnswer,
		},
		longAnswers: map[common.Address][]byte{
			candidate: {57, 99}, // 11100101100011
		},
		epochDb: epochDb,
	}

	// when
	shortPoint, shortQualifiedFlipsCount := q.qualifyCandidate(candidate, flipQualificationMap, flipsToSolve, true, notApprovedFlips)
	longPoint, longQualifiedFlipsCount := q.qualifyCandidate(candidate, flipQualificationMap, flipsToSolve, false, notApprovedFlips)

	mShortPoint, mShortQualifiedFlipsCount := q.qualifyCandidate(maliciousCandidate, flipQualificationMap, flipsToSolve, true, notApprovedFlips)

	// then
	require.Equal(t, float32(3.5), shortPoint)
	require.Equal(t, uint32(6), shortQualifiedFlipsCount)
	require.Equal(t, float32(3.5), longPoint)
	require.Equal(t, uint32(7), longQualifiedFlipsCount)

	require.Equal(t, float32(0), mShortPoint)
	require.Equal(t, uint32(len(flipsToSolve)), mShortQualifiedFlipsCount)
}

func fillArray(left int, right int, inapp int, none int) []types.Answer {
	var ans []types.Answer

	for i := 0; i < left; i++ {
		ans = append(ans, types.Left)
	}
	for i := 0; i < right; i++ {
		ans = append(ans, types.Right)
	}
	for i := 0; i < inapp; i++ {
		ans = append(ans, types.Inappropriate)
	}
	for i := 0; i < none; i++ {
		ans = append(ans, types.None)
	}

	rand.Shuffle(len(ans), func(i, j int) {
		ans[i], ans[j] = ans[j], ans[i]
	})

	return ans
}
