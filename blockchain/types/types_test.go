package types

import (
	"gx/ipfs/QmPVkJMTeRC6iBByPWdrRkD3BE5UXsj5HPzb4kPqL186mS/testify/require"
	"testing"
)

func TestAnswers_Answer(t *testing.T) {

	require := require.New(t)

	answers := NewAnswers(11)

	answers.Right(0)

	answers.Right(3)
	answers.Inappropriate(3)

	answers.Left(9)
	answers.Inappropriate(9)
	answers.Easy(9)

	answer, inapp, easy := answers.Answer(0)
	require.True(answer == Right && !inapp && !easy)

	answer, inapp, easy = answers.Answer(3)
	require.True(answer == Right && inapp && !easy)

	answer, inapp, easy = answers.Answer(9)
	require.True(answer == Left && inapp && easy)

	answer, inapp, easy = answers.Answer(10)
	require.True(answer == None && !inapp && !easy)
}
