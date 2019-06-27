package api

import (
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/core/ceremony"
	"idena-go/core/flip"
	"idena-go/core/state"
	"idena-go/ipfs"
	"idena-go/protocol"
)

const (
	MaxFlipSize = 1024 * 600
)

type FlipApi struct {
	baseApi   *BaseApi
	fp        *flip.Flipper
	pm        *protocol.ProtocolManager
	ipfsProxy ipfs.Proxy
	ceremony  *ceremony.ValidationCeremony
}

// NewFlipApi creates a new FlipApi instance
func NewFlipApi(baseApi *BaseApi, fp *flip.Flipper, pm *protocol.ProtocolManager, ipfsProxy ipfs.Proxy, ceremony *ceremony.ValidationCeremony) *FlipApi {
	return &FlipApi{baseApi, fp, pm, ipfsProxy, ceremony}
}

type FlipSubmitResponse struct {
	TxHash common.Hash `json:"txHash"`
	Hash   string      `json:"hash"`
}

// SubmitFlip receives an image as hex
func (api *FlipApi) Submit(hex *hexutil.Bytes) (FlipSubmitResponse, error) {

	if hex == nil {
		return FlipSubmitResponse{}, errors.New("flip is empty")
	}

	rawFlip := *hex

	if len(rawFlip) > MaxFlipSize {
		return FlipSubmitResponse{}, errors.Errorf("flip is too big, max expected size %v, actual %v", MaxFlipSize, len(rawFlip))
	}

	cid, encryptedFlip, err := api.fp.PrepareFlip(rawFlip)

	if err != nil {
		return FlipSubmitResponse{}, err
	}

	addr := api.baseApi.getCurrentCoinbase()

	tx, err := api.baseApi.getSignedTx(addr, nil, types.SubmitFlipTx, decimal.Zero, 0, 0, cid.Bytes(), nil)

	if err != nil {
		return FlipSubmitResponse{}, err
	}

	flip := types.Flip{
		Tx:   tx,
		Data: encryptedFlip,
	}

	if err := api.fp.AddNewFlip(flip); err != nil {
		return FlipSubmitResponse{}, err
	}

	api.pm.BroadcastFlip(&flip)

	return FlipSubmitResponse{
		TxHash: tx.Hash(),
		Hash:   cid.String(),
	}, nil
}

type FlipHashesResponse struct {
	Hash  string `json:"hash"`
	Ready bool   `json:"ready"`
}

func (api *FlipApi) ShortHashes() ([]FlipHashesResponse, error) {
	period := api.baseApi.getAppState().State.ValidationPeriod()

	if period != state.FlipLotteryPeriod && period != state.ShortSessionPeriod {
		return nil, errors.New("this method is available during FlipLottery and ShortSession periods")
	}

	if !api.ceremony.IsCandidate() {
		return nil, errors.New("coinbase address is not a ceremony candidate")
	}

	flips := api.ceremony.GetShortFlipsToSolve()

	return prepareHashes(api.fp, flips)
}

func (api *FlipApi) LongHashes() ([]FlipHashesResponse, error) {
	period := api.baseApi.getAppState().State.ValidationPeriod()

	if period != state.FlipLotteryPeriod && period != state.ShortSessionPeriod && period != state.LongSessionPeriod {
		return nil, errors.New("this method is available during FlipLottery, ShortSession and LongSession periods")
	}

	if !api.ceremony.IsCandidate() {
		return nil, errors.New("coinbase address is not a ceremony candidate")
	}

	flips := api.ceremony.GetLongFlipsToSolve()

	return prepareHashes(api.fp, flips)
}

func prepareHashes(flipper *flip.Flipper, flips [][]byte) ([]FlipHashesResponse, error) {
	if flips == nil {
		return nil, errors.New("no flips to solve")
	}

	var result []FlipHashesResponse
	for _, v := range flips {
		cid, _ := cid.Parse(v)
		result = append(result, FlipHashesResponse{
			Hash:  cid.String(),
			Ready: flipper.IsFlipReady(v),
		})
	}

	return result, nil
}

type FlipResponse struct {
	Hex hexutil.Bytes `json:"hex"`
}

func (api *FlipApi) Get(hash string) (FlipResponse, error) {
	c, err := cid.Decode(hash)
	if err != nil {
		return FlipResponse{}, err
	}
	cidBytes := c.Bytes()

	data, err := api.fp.GetFlip(cidBytes)

	if err != nil {
		return FlipResponse{}, err
	}

	return FlipResponse{
		Hex: hexutil.Bytes(data),
	}, nil
}

type FlipAnswer struct {
	Easy   bool         `json:"easy"`
	Answer types.Answer `json:"answer"`
}

type SubmitAnswersArgs struct {
	Answers []FlipAnswer `json:"answers"`
	Nonce   uint32       `json:"nonce"`
	Epoch   uint16       `json:"epoch"`
}

type SubmitAnswersResponse struct {
	TxHash common.Hash `json:"txHash"`
}

func (api *FlipApi) SubmitShortAnswers(args SubmitAnswersArgs) (SubmitAnswersResponse, error) {
	if !api.ceremony.IsCandidate() {
		return SubmitAnswersResponse{}, errors.New("coinbase address is not a ceremony candidate")
	}

	flipsCount := len(api.ceremony.GetShortFlipsToSolve())
	if flipsCount != len(args.Answers) {
		return SubmitAnswersResponse{}, errors.Errorf("some answers are missing, expected %v, actual %v", flipsCount, len(args.Answers))
	}

	answers := parseAnswers(args.Answers, uint(flipsCount))

	hash, err := api.ceremony.SubmitShortAnswers(answers)

	if err != nil {
		return SubmitAnswersResponse{}, err
	}

	return SubmitAnswersResponse{
		TxHash: hash,
	}, nil
}

func (api *FlipApi) SubmitLongAnswers(args SubmitAnswersArgs) (SubmitAnswersResponse, error) {
	if !api.ceremony.IsCandidate() {
		return SubmitAnswersResponse{}, errors.New("coinbase address is not a ceremony candidate")
	}

	flipsCount := len(api.ceremony.GetLongFlipsToSolve())
	if flipsCount != len(args.Answers) {
		return SubmitAnswersResponse{}, errors.Errorf("some answers are missing, expected %v, actual %v", flipsCount, len(args.Answers))
	}

	answers := parseAnswers(args.Answers, uint(flipsCount))

	hash, err := api.ceremony.SubmitLongAnswers(answers)

	if err != nil {
		return SubmitAnswersResponse{}, err
	}

	return SubmitAnswersResponse{
		TxHash: hash,
	}, nil
}

func parseAnswers(answers []FlipAnswer, flipsCount uint) *types.Answers {
	result := types.NewAnswers(flipsCount)

	for i := uint(0); i < uint(len(answers)); i++ {
		item := answers[i]
		if item.Answer == types.None {
			continue
		}
		if item.Answer == types.Left {
			result.Left(i)
		}
		if item.Answer == types.Right {
			result.Right(i)
		}
		if item.Answer == types.Inappropriate {
			result.Inappropriate(i)
		}
		if item.Easy {
			result.Easy(i)
		}
	}

	return result
}
