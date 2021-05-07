package common

const minShardSize = 1400
const maxShardSize = 3000

func CalculateShardsNumber(networkSize, currentShardsNum int) int {
	for {
		shouldRemoveShards := networkSize <= minShardSize*currentShardsNum
		shouldAddShards := networkSize >= maxShardSize*currentShardsNum
		if !shouldAddShards && !shouldRemoveShards || shouldRemoveShards && currentShardsNum == 1 {
			return currentShardsNum
		}
		if shouldAddShards {
			currentShardsNum *= 2
		}
		if shouldRemoveShards {
			currentShardsNum /= 2
		}
	}
}
