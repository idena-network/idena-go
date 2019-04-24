package ceremony

import (
	"github.com/stretchr/testify/require"
	"idena-go/blockchain/types"
	"math/rand"
	"testing"
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
