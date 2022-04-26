package types

import (
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestAnswers_Answer(t *testing.T) {

	require := require.New(t)

	answers := NewAnswers(11)

	answers.Right(0)
	answers.Grade(0, GradeA)

	answers.Grade(1, GradeB)

	answers.Grade(2, GradeC)

	answers.Left(3)
	answers.Grade(3, GradeReported)

	answers.Right(4)
	invalidGrade := 6
	invalidGradeBig := big.NewInt(int64(invalidGrade))
	answers.Bits.Or(answers.Bits, invalidGradeBig.Lsh(invalidGradeBig, 4*3+answers.FlipsCount*2))

	answers.Left(9)
	answers.Grade(9, GradeD)

	answer, grade := answers.Answer(0)
	require.True(answer == Right && grade == GradeA)

	answer, grade = answers.Answer(1)
	require.True(answer == None && grade == GradeB)

	answer, grade = answers.Answer(2)
	require.True(answer == None && grade == GradeC)

	answer, grade = answers.Answer(3)
	require.True(answer == Left && grade == GradeReported)

	answer, grade = answers.Answer(4)
	require.True(answer == Right && grade == GradeNone)

	answer, grade = answers.Answer(9)
	require.True(answer == Left && grade == GradeD)

	answer, grade = answers.Answer(10)
	require.True(answer == None && grade == GradeNone)

	reports, approvals := answers.Grades()
	require.Equal(1, reports)
	require.Equal(4, approvals)
}

func TestBlockFlag_HasFlag(t *testing.T) {
	var flags BlockFlag
	flags = flags | IdentityUpdate
	flags = flags | Snapshot

	require.True(t, flags.HasFlag(IdentityUpdate))
	require.True(t, flags.HasFlag(Snapshot))

	require.True(t, flags.HasFlag(Snapshot|IdentityUpdate))

	require.False(t, flags.HasFlag(FlipLotteryStarted))
	require.False(t, flags.HasFlag(ShortSessionStarted))
	require.False(t, flags.HasFlag(LongSessionStarted))
	require.False(t, flags.HasFlag(AfterLongSessionStarted))
}

func TestBlockCert_Empty(t *testing.T) {
	var cert *BlockCert
	require.True(t, cert.Empty())
}
