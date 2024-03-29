package ceremony

import (
	"container/list"
	"encoding/binary"
	"github.com/idena-network/idena-go/common"
	"math/rand"
)

const (
	CandidatesPerAuthor = 13
)

type shardAuthors struct {
	authorsPerCandidate map[int][]int
	candidatesPerAuthor map[int][]int
}

type flipDistribution struct {
	shortFlipsPerCandidate [][]int
	longFlipsPerCandidate [][]int
}

func GetAuthorsDistribution(shards map[common.ShardId]*candidatesOfShard, seed []byte, shortFlipsCount int) map[common.ShardId]*shardAuthors {

	shardLotteries := make(map[common.ShardId]*shardAuthors)

	for shardId, shard := range shards {
		if len(shard.candidates) == 0 {
			shardLotteries[shardId] = &shardAuthors{
				authorsPerCandidate: make(map[int][]int),
				candidatesPerAuthor: make(map[int][]int),
			}
			continue
		}

		authors := getAuthorsIndexes(shard.candidates)

		if len(authors) == 0 {
			shardLotteries[shardId] = &shardAuthors{
				authorsPerCandidate: make(map[int][]int),
				candidatesPerAuthor: make(map[int][]int),
			}
			continue
		}

		authorsPerCandidate, candidatesPerAuthor := getFirstAuthorsDistribution(authors, shard.candidates, seed, shortFlipsCount)

		if len(authors) > 7 {
			authorsPerCandidate, candidatesPerAuthor = appendAdditionalCandidates(seed, shard.candidates, authorsPerCandidate, candidatesPerAuthor)
		}
		shardLotteries[shardId] = &shardAuthors{
			authorsPerCandidate: authorsPerCandidate,
			candidatesPerAuthor: candidatesPerAuthor,
		}
	}
	return shardLotteries
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
		suitableAuthor := getNextSuitablePair(queue, candidateIndex, authorsPerCandidate[candidateIndex])

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
		if currentCount >= CandidatesPerAuthor {
			continue
		}

		if candidatesQueue.Len() == 0 {
			candidatesQueue = getRandomizedCandidates()
		}

		// do until each candidate has 11 candidates
		for currentCount < CandidatesPerAuthor {
			if candidatesQueue.Len() == 0 {
				break
			}

			// for current author try to find suitable candidate
			suitableCandidate := getNextSuitablePair(candidatesQueue, author, candidatesPerAuthor[author])

			currentCount++
			candidatesPerAuthor[author] = append(candidatesPerAuthor[author], suitableCandidate)
			authorsPerCandidate[suitableCandidate] = append(authorsPerCandidate[suitableCandidate], author)
		}
	}

	return authorsPerCandidate, candidatesPerAuthor
}

func GetFlipsDistribution(candidatesCount int, authorsPerCandidate map[int][]int, flipsPerAuthor map[int][][]byte, flips [][]byte, seed []byte, shortFlipsCount int) (shortFlipsPerCandidate [][]int, longFlipsPerCandidate  [][]int) {
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

	usedAuthors := make([]int, candidatesCount)
	currentFlipIndexByAuthor := make([]int, candidatesCount)
	hashMap := make(map[string]int)
	for idx, item := range flips {
		hashMap[string(item)] = idx
	}

	randSeed := binary.LittleEndian.Uint64(seed)
	random := rand.New(rand.NewSource(int64(randSeed)*12 + 3))
	permutation := random.Perm(candidatesCount)

	getMinUsedAuthor := func(a []int, localUsedAuthors map[int]bool) int {
		min := 999999
		author := 0
		for _, item := range a {
			if _, ok := localUsedAuthors[item]; ok {
				continue
			}
			if usedAuthors[item] < min {
				author = item
				min = usedAuthors[item]
			}
		}
		return author
	}

	addFlip := func(arr []int, f []byte) []int {
		flipGlobalIndex := hashMap[string(f)]
		return append(arr, flipGlobalIndex)
	}

	shortFlipsPerCandidate = make([][]int, candidatesCount)
	longFlipsPerCandidate = make([][]int, candidatesCount)

	chooseNextShortFlip := func(authors []int, localUsedAuthors map[int]bool) (author int, flip []byte, idx int) {
		author = getMinUsedAuthor(authors, localUsedAuthors)
		authorFlips := flipsPerAuthor[author]
		currentAuthorFlipIdx := currentFlipIndexByAuthor[author]

		// all authors already used, need to clear and search again
		if currentAuthorFlipIdx >= len(authorFlips) {
			currentFlipIndexByAuthor[author] = 0
			currentAuthorFlipIdx = currentFlipIndexByAuthor[author]
		}
		currentFlipIndexByAuthor[author] += 1

		return author, authorFlips[currentAuthorFlipIdx], currentAuthorFlipIdx
	}

	for _, candidate := range permutation {

		authors, ok := authorsPerCandidate[candidate]
		if !ok {
			continue
		}

		authors = distinct(authors)

		var shortFlips []int
		var longFlips []int

		if len(authors) < shortFlipsCount {
			localUsedAuthors := make(map[int]bool)
			for j := 0; j < shortFlipsCount; j++ {
				if len(localUsedAuthors) == len(authors) {
					localUsedAuthors = make(map[int]bool)
				}
				author, nextFlip, _ := chooseNextShortFlip(authors, localUsedAuthors)
				shortFlips = addFlip(shortFlips, nextFlip)
				usedAuthors[author] += 1
				localUsedAuthors[author] = true
			}

			// add all flips to long session
			for _, author := range authors {
				authorFlips := flipsPerAuthor[author]
				for _, f := range authorFlips {
					longFlips = addFlip(longFlips, f)
				}
			}
		} else {
			usedFlipIndexes := make(map[int]map[int]bool)
			localUsedAuthors := make(map[int]bool)
			for j := 0; j < shortFlipsCount; j++ {

				author, nextFlip, index := chooseNextShortFlip(authors, localUsedAuthors)
				if _, ok := usedFlipIndexes[author]; !ok {
					usedFlipIndexes[author] = make(map[int]bool)
				}
				usedFlipIndexes[author][index] = true
				shortFlips = addFlip(shortFlips, nextFlip)

				usedAuthors[author] += 1
				localUsedAuthors[author] = true
			}
			// need to add all flips from unused authors to long session
			// and append flips, which was not chosen for short session
			for _, author := range authors {
				authorFlips := flipsPerAuthor[author]
				if u, ok := usedFlipIndexes[author]; !ok {
					for _, f := range authorFlips {
						longFlips = addFlip(longFlips, f)
					}
				} else {
					for idx, f := range authorFlips {
						if !u[idx] {
							longFlips = addFlip(longFlips, f)
						}
					}
				}
			}
		}

		shortFlipsPerCandidate[candidate] = distinct(shortFlips)
		longFlipsPerCandidate[candidate] = distinct(longFlips)
	}

	// case when only 1 flip is available
	for i := 0; i < len(longFlipsPerCandidate); i++ {
		if len(longFlipsPerCandidate[i]) == 0 {
			longFlipsPerCandidate[i] = []int{0}
		}
	}
	return shortFlipsPerCandidate, longFlipsPerCandidate
}

func getNextSuitablePair(indexesQueue Queue, currentCandidate int, currentCandidateUsedIndexes []int) (suitableCandidatePair int) {
	for i := 0; i < indexesQueue.Len(); i++ {
		nextCandidate := indexesQueue.Pop()
		if currentCandidate != nextCandidate && !contains(currentCandidateUsedIndexes, nextCandidate) {
			return nextCandidate
		} else {
			indexesQueue.Push(nextCandidate)
		}
	}

	nextCandidate := indexesQueue.Pop()
	return nextCandidate
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
