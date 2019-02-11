package api

import (
	"github.com/pkg/errors"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/core/flip"
)

const (
	MaxFlipSize = 1024 * 150
)

type FlipApi struct {
	baseApi   *BaseApi
	flipStore flip.Store
}

// NewFlipApi creates a new FlipApi instance
func NewFlipApi(baseApi *BaseApi, flipStore flip.Store) *FlipApi {
	return &FlipApi{baseApi, flipStore}
}

type Flip struct {
	Category uint16         `json:"category"`
	Left     *hexutil.Bytes `json:"left"`
	Right    *hexutil.Bytes `json:"right"`
}

type FlipSubmitResponse struct {
	TxHash   common.Hash `json:"txHash"`
	FlipHash common.Hash `json:"flipHash"`
}

// SubmitFlip receives an image as hex
func (api *FlipApi) SubmitFlip(data Flip) (FlipSubmitResponse, error) {

	if data.Left == nil || data.Right == nil {
		return FlipSubmitResponse{}, errors.New("flip is empty")
	}

	leftFlip := *data.Left
	rightFlip := *data.Right

	if len(leftFlip) > MaxFlipSize {
		return FlipSubmitResponse{}, errors.Errorf("left flip is too big, max expected size %v, actual %v", MaxFlipSize, len(leftFlip))
	}

	if len(rightFlip) > MaxFlipSize {
		return FlipSubmitResponse{}, errors.Errorf("right flip is too big, max expected size %v, actual %v", MaxFlipSize, len(rightFlip))
	}

	epoch := api.baseApi.engine.GetAppState().State.Epoch()

	hash, err := api.flipStore.PrepareFlip(epoch, data.Category, leftFlip, rightFlip)

	if err != nil {
		return FlipSubmitResponse{}, err
	}

	txHash, err := api.baseApi.sendTx(api.baseApi.getCurrentCoinbase(), common.Address{}, types.SubmitFlipTx, nil, 0, 0, hash[:], nil)

	// TODO: p2p send flip

	if err != nil {
		return FlipSubmitResponse{}, err
	}

	return FlipSubmitResponse{
		TxHash:   txHash,
		FlipHash: hash,
	}, nil
}

type FlipResponse struct {
	Category uint16        `json:"category"`
	Left     hexutil.Bytes `json:"left"`
	Right    hexutil.Bytes `json:"right"`
	Epoch    uint16        `json:"epoch"`
	Mined    bool          `json:"mined"`
}

func (api *FlipApi) GetFlip(hash common.Hash) (FlipResponse, error) {
	flip, err := api.flipStore.GetFlip(hash)

	if err != nil {
		return FlipResponse{}, err
	}

	return FlipResponse{
		Mined:    flip.Mined,
		Epoch:    flip.Epoch,
		Left:     hexutil.Bytes(flip.Data.Left),
		Right:    hexutil.Bytes(flip.Data.Right),
		Category: flip.Data.Category,
	}, nil
}
