package ceremony

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/rlp"
	"math/rand"
)

const (
	GeneticRelationLength = 3
)

func SortFlips(flipsPerAuthor map[int][][]byte, candidates []*candidate, flips [][]byte, seed []byte, shortFlipsCount int) (shortFlipsPerCandidate [][]int, longFlipsPerCandidate [][]int) {
	if len(candidates) == 0 {
		return shortFlipsPerCandidate, longFlipsPerCandidate
	}

	authors := getAuthorsIndexes(candidates)

	if len(authors) == 0 {
		return make([][]int, len(candidates)), make([][]int, len(candidates))
	}

	authorsPerCandidate, candidatesPerAuthor := getFirstAuthorsDistribution(authors, candidates, seed, shortFlipsCount)

	if len(authors) > 7 {
		authorsPerCandidate, candidatesPerAuthor = appendAdditionalCandidates(seed, candidates, authorsPerCandidate, candidatesPerAuthor)
	}

	return determineFlips(authorsPerCandidate, flipsPerAuthor, flips, seed, shortFlipsCount)
}

func getFirstAuthorsDistribution(authorsIndexes []int, candidates []*candidate, seed []byte, shortFlipsCount int) (authorsPerCandidate map[int][]int, candidatesPerAuthor map[int][]int) {
	queue := fillAuthorsQueue(seed, authorsIndexes, candidates, shortFlipsCount)
	candidateIndex := 0
	authorsPerCandidate = make(map[int][]int)
	candidatesPerAuthor = make(map[int][]int)

	for {
		if queue.Len() == 0 {
			break
		}
		if candidateIndex == len(candidates) {
			candidateIndex = 0
		}

		// for current candidate try to find suitable author
		suitableAuthor := getNextSuitablePair(candidates, queue, candidateIndex, authorsPerCandidate[candidateIndex])

		authorsPerCandidate[candidateIndex] = append(authorsPerCandidate[candidateIndex], suitableAuthor)
		candidatesPerAuthor[suitableAuthor] = append(candidatesPerAuthor[suitableAuthor], candidateIndex)

		candidateIndex++
	}

	return authorsPerCandidate, candidatesPerAuthor
}

func appendAdditionalCandidates(seed []byte, candidates []*candidate, authorsPerCandidate map[int][]int, candidatesPerAuthor map[int][]int) (map[int][]int, map[int][]int) {
	randSeed := binary.LittleEndian.Uint64(seed)
	random := rand.New(rand.NewSource(int64(randSeed)*77 + 55))

	getRandomizedCandidates := func() Queue {
		p := random.Perm(len(candidates))
		queue := NewQueue()
		for _, idx := range p {
			queue.Push(idx)
		}
		return queue
	}

	candidatesQueue := getRandomizedCandidates()

	for author := 0; author < len(candidates); author++ {
		value, ok := candidatesPerAuthor[author]
		if !ok {
			continue
		}
		currentCount := len(value)
		if currentCount >= 11 {
			continue
		}

		if candidatesQueue.Len() == 0 {
			candidatesQueue = getRandomizedCandidates()
		}

		// do until each candidate has 11 candidates
		for currentCount < 11 {
			if candidatesQueue.Len() == 0 {
				break
			}

			// for current author try to find suitable candidate
			suitableCandidate := getNextSuitablePair(candidates, candidatesQueue, author, candidatesPerAuthor[author])

			currentCount++
			candidatesPerAuthor[author] = append(candidatesPerAuthor[author], suitableCandidate)
			authorsPerCandidate[suitableCandidate] = append(authorsPerCandidate[suitableCandidate], author)
		}
	}

	return authorsPerCandidate, candidatesPerAuthor
}

func determineFlips(authorsPerCandidate map[int][]int, flipsPerAuthor map[int][][]byte, flips [][]byte, seed []byte, shortFlipsCount int) (shortFlipsPerCandidate [][]int, longFlipsPerCandidate [][]int) {

	distinct := func(arr []int) []int {
		m := make(map[int]struct{})
		var output []int
		for _, item := range arr {
			if _, ok := m[item]; ok {
				continue
			}
			output = append(output, item)
			m[item] = struct{}{}
		}
		return output
	}

	hashMap := make(map[common.Hash]int)
	for idx, item := range flips {
		hashMap[common.Hash(rlp.Hash(item))] = idx
	}
	randSeed := binary.LittleEndian.Uint64(seed)
	random := rand.New(rand.NewSource(int64(randSeed)*12 + 3))

	shortFlipsPerCandidate = make([][]int, len(authorsPerCandidate))
	longFlipsPerCandidate = make([][]int, len(authorsPerCandidate))

	for candidateIndex := 0; candidateIndex < len(authorsPerCandidate); candidateIndex++ {
		authors := authorsPerCandidate[candidateIndex]
		randomizedAuthors := random.Perm(len(authors))
		var shortFlips []int
		var longFlips []int
		for j := 0; j < len(authors); j++ {
			author := authors[randomizedAuthors[j]]
			flips := flipsPerAuthor[author]
			if len(shortFlips) < shortFlipsCount {
				flipGlobalIndex := -1
				for i := 0; i < len(flips); i++ {
					randomIndex := random.Intn(len(flips))
					flipHash := common.Hash(rlp.Hash(flips[randomIndex]))
					flipGlobalIndex = hashMap[flipHash]
					if !contains(shortFlips, flipGlobalIndex) {
						break
					}
				}

				shortFlips = append(shortFlips, flipGlobalIndex)
			}

			for k := 0; k < len(flips); k++ {
				flipHash := common.Hash(rlp.Hash(flips[k]))
				flipGlobalIndex := hashMap[flipHash]
				if !contains(shortFlips, flipGlobalIndex) {
					longFlips = append(longFlips, flipGlobalIndex)
				}
			}
		}
		shortFlipsPerCandidate[candidateIndex] = distinct(shortFlips)
		longFlipsPerCandidate[candidateIndex] = distinct(longFlips)
	}

	// case when only 1 flip is available
	for i := 0; i < len(longFlipsPerCandidate); i++ {
		if len(longFlipsPerCandidate[i]) == 0 {
			longFlipsPerCandidate[i] = []int{0}
		}
	}
	return shortFlipsPerCandidate, longFlipsPerCandidate
}

func getNextSuitablePair(candidates []*candidate, indexesQueue Queue, currentCandidate int, currentCandidateUsedIndexes []int) (suitableCandidatePair int) {

	// first traversal (with relation)
	for i := 0; i < indexesQueue.Len(); i++ {
		nextCandidate := indexesQueue.Pop()
		if checkIfCandidateSuits(candidates, currentCandidate, nextCandidate, currentCandidateUsedIndexes, hasRelation) {
			return nextCandidate
		} else {
			indexesQueue.Push(nextCandidate)
		}
	}

	// second traversal (without relation)
	for i := 0; i < indexesQueue.Len(); i++ {
		nextCandidate := indexesQueue.Pop()
		if checkIfCandidateSuits(candidates, currentCandidate, nextCandidate, currentCandidateUsedIndexes, func(first *candidate, second *candidate, geneticOverlapLength int) bool {
			return false
		}) {
			return nextCandidate
		} else {
			indexesQueue.Push(nextCandidate)
		}
	}

	nextCandidate := indexesQueue.Pop()
	return nextCandidate
}

func checkIfCandidateSuits(candidates []*candidate, currentCandidate int, nextCandidate int, currentCandidateUsedIndexes []int, hasRelationFn func(first *candidate, second *candidate, geneticOverlapLength int) bool) bool {
	return currentCandidate != nextCandidate &&
		!hasRelationFn(candidates[currentCandidate], candidates[nextCandidate], GeneticRelationLength) &&
		!contains(currentCandidateUsedIndexes, nextCandidate)
}

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func fillAuthorsQueue(seed []byte, authorsIndexes []int, candidates []*candidate, authorsPerCandidate int) Queue {
	totalAuthorsShouldBe := len(candidates) * authorsPerCandidate
	randSeed := binary.LittleEndian.Uint64(seed)
	random := rand.New(rand.NewSource(int64(randSeed)*21 - 77))
	randomizedAuthors := random.Perm(len(authorsIndexes))
	queue := NewQueue()
	if len(authorsIndexes) == 0 {
		return queue
	}
	idx := 0
	for {
		if queue.Len() == totalAuthorsShouldBe {
			break
		}
		if idx == len(authorsIndexes) {
			idx = 0
			randomizedAuthors = random.Perm(len(authorsIndexes))
		}
		queue.Push(authorsIndexes[randomizedAuthors[idx]])
		idx++
	}
	return queue
}

func getAuthorsIndexes(candidates []*candidate) []int {
	var authors []int
	for idx, candidate := range candidates {
		if candidate.IsAuthor {
			authors = append(authors, idx)
		}
	}
	return authors
}

func hasRelation(first *candidate, second *candidate, geneticOverlapLength int) bool {

	res := func(a *candidate, b *candidate) bool {
		codeLength := len(a.Code)
		diff := b.Generation - a.Generation
		if diff > uint32(codeLength-geneticOverlapLength) {
			return false
		}

		return bytes.Compare(a.Code[diff:][:geneticOverlapLength], b.Code[:geneticOverlapLength]) == 0
	}

	if first.Generation <= second.Generation {
		return res(first, second)
	} else {
		return res(second, first)
	}
}

// Queue is a queue
type Queue interface {
	Pop() int
	Push(int)
	Peek() int
	Len() int
}

type queueImpl struct {
	*list.List
}

func (q *queueImpl) Push(v int) {
	q.PushBack(v)
}

func (q *queueImpl) Pop() int {
	e := q.Front()
	q.List.Remove(e)
	return e.Value.(int)
}

func (q *queueImpl) Peek() int {
	e := q.Front()
	return e.Value.(int)
}

func NewQueue() Queue {
	return &queueImpl{list.New()}
}
