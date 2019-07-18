package types

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAnswers_Answer(t *testing.T) {

	require := require.New(t)

	answers := NewAnswers(11)

	answers.Right(0)

	answers.Inappropriate(3)

	answers.Left(9)
	answers.Easy(9)

	answer, easy := answers.Answer(0)
	require.True(answer == Right && !easy)

	answer, easy = answers.Answer(3)
	require.True(answer == Inappropriate && !easy)

	answer, easy = answers.Answer(9)
	require.True(answer == Left && easy)

	answer, easy = answers.Answer(10)
	require.True(answer == None && !easy)
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
