package common

const MinShardSize = 2200
const MaxShardSize = 4600

func CalculateShardsNumber(minShardSize, maxShardSize, networkSize, currentShardsNum int) int {
	shouldRemoveShards := networkSize <= minShardSize*currentShardsNum
	shouldAddShards := networkSize >= maxShardSize*currentShardsNum

	for shouldAddShards {
		currentShardsNum *= 2
		if networkSize < maxShardSize*currentShardsNum {
			return currentShardsNum
		}
	}
	for shouldRemoveShards && currentShardsNum > 1 {
		currentShardsNum /= 2
		if networkSize > minShardSize*currentShardsNum || currentShardsNum == 1 {
			return currentShardsNum
		}
	}
	return currentShardsNum
}
