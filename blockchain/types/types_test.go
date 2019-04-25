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

	answer,  easy := answers.Answer(0)
	require.True(answer == Right && !easy)

	answer, easy = answers.Answer(3)
	require.True(answer == Inappropriate && !easy)

	answer,  easy = answers.Answer(9)
	require.True(answer == Left && easy)

	answer, easy = answers.Answer(10)
	require.True(answer == None && !easy)
}
