package ceremony

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tm-db"
	"math/rand"
	"testing"
	"time"
)

func Test_getAnswersCount(t *testing.T) {
	require := require.New(t)

	ans := fillArray(10, 13, 4, 3)

	resLeft, resRight, none, resInapp := getAnswersCount(ans)

	require.Equal(uint(10), resLeft)
	require.Equal(uint(13), resRight)
	require.Equal(uint(4), resInapp)
	require.Equal(uint(3), none)
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

	ans = fillArray(1, 1, 1, 10)
	q = qualifyOneFlip(ans)
	require.Equal(QualifiedByNone, q.status)
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

	shortFlipsToSolve := []int{10, 11, 12, 13, 20, 21, 22}
	longFlipsToSolve := []int{10, 11, 12, 13, 20, 21, 22, 23, 24}

	flipQualificationMap := make(map[int]FlipQualification)
	flipQualificationMap[10] = FlipQualification{
		status: Qualified,
		answer: types.Left,
	}
	flipQualificationMap[11] = FlipQualification{
		status: NotQualified,
		answer: types.None,
	}
	flipQualificationMap[12] = FlipQualification{
		status: NotQualified,
		answer: types.None,
	}
	flipQualificationMap[13] = FlipQualification{
		status: NotQualified,
		answer: types.None,
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
	notApprovedFlips.Add(11)
	notApprovedFlips.Add(12)

	candidate := tests.GetRandAddr()
	maliciousCandidate := tests.GetRandAddr()

	short := types.NewAnswers(uint(len(shortFlipsToSolve)))
	short.Left(0)
	short.Right(3)
	short.Right(4)
	short.Left(5)
	short.Inappropriate(6)

	long := types.NewAnswers(uint(len(longFlipsToSolve)))
	long.Left(0)
	long.Right(3)
	long.Right(4)
	long.Left(5)
	long.Inappropriate(6)
	long.Left(7)
	long.Right(8)

	shortAnswer := short.Bytes()
	salt := []byte{0x1, 0x10, 0x25}
	epochDb.WriteAnswerHash(candidate, rlp.Hash(append(shortAnswer, salt...)), time.Now())
	epochDb.WriteAnswerHash(maliciousCandidate, rlp.Hash([]byte{0x0}), time.Now())
	key, _ := crypto.GenerateKey()
	attachment := attachments.CreateShortAnswerAttachment(shortAnswer, nil, salt, ecies.ImportECDSA(key))
	maliciousAttachment := attachments.CreateShortAnswerAttachment(shortAnswer, nil, []byte{0x6}, ecies.ImportECDSA(key))
	q := qualification{
		shortAnswers: map[common.Address][]byte{
			candidate:          attachment,
			maliciousCandidate: maliciousAttachment,
		},
		longAnswers: map[common.Address][]byte{
			candidate: long.Bytes(),
		},
		epochDb: epochDb,
	}

	// when
	shortPoint, shortQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(candidate, flipQualificationMap, shortFlipsToSolve, true, notApprovedFlips)
	longPoint, longQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(candidate, flipQualificationMap, longFlipsToSolve, false, notApprovedFlips)

	mShortPoint, mShortQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(maliciousCandidate, flipQualificationMap, shortFlipsToSolve, true, notApprovedFlips)

	// then
	require.Equal(t, float32(3.5), shortPoint)
	require.Equal(t, uint32(4), shortQualifiedFlipsCount)
	require.Equal(t, float32(4.5), longPoint)
	require.Equal(t, uint32(6), longQualifiedFlipsCount)

	require.Equal(t, float32(0), mShortPoint)
	require.Equal(t, uint32(6), mShortQualifiedFlipsCount)
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
	}
	flipQualificationMap[55] = FlipQualification{
		status: WeaklyQualified,
		answer: types.Left,
	}

	short := types.NewAnswers(uint(len(shortFlipsToSolve)))
	short.Left(0)
	short.Right(1)

	shortAnswer := short.Bytes()
	salt := []byte{0x1, 0x10, 0x25, 0x28}
	epochDb.WriteAnswerHash(candidate, rlp.Hash(append(shortAnswer, salt...)), time.Now())
	key, _ := crypto.GenerateKey()
	attachment := attachments.CreateShortAnswerAttachment(shortAnswer, nil, salt, ecies.ImportECDSA(key))
	q := qualification{
		shortAnswers: map[common.Address][]byte{
			candidate: attachment,
		},
		epochDb: epochDb,
	}
	shortPoint, shortQualifiedFlipsCount, _, _, _ := q.qualifyCandidate(candidate, flipQualificationMap, shortFlipsToSolve, true, mapset.NewSet())

	require.Equal(t, float32(1.5), shortPoint)
	require.Equal(t, uint32(2), shortQualifiedFlipsCount)

	// wrong salt
	epochDb.Clear()
	wrongSalt := []byte{0x1}
	epochDb.WriteAnswerHash(candidate, rlp.Hash(append(shortAnswer, wrongSalt...)), time.Now())
	q2 := qualification{
		shortAnswers: map[common.Address][]byte{
			candidate: attachment,
		},
		epochDb: epochDb,
	}
	shortPoint, shortQualifiedFlipsCount, _, _, _ = q2.qualifyCandidate(candidate, flipQualificationMap, shortFlipsToSolve, true, mapset.NewSet())

	require.Equal(t, float32(0), shortPoint)
	require.Equal(t, uint32(2), shortQualifiedFlipsCount)
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
