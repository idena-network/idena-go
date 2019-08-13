package api

import (
	"bytes"
	"encoding/json"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/ceremony"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/protocol"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
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

type FlipSubmitArgs struct {
	Hex  *hexutil.Bytes `json:"hex"`
	Pair uint8          `json:"pair"`
}

func (api *FlipApi) Submit(i *json.RawMessage) (FlipSubmitResponse, error) {
	//TODO: remove this after desktop updating
	// temp code start
	args := &FlipSubmitArgs{}
	dec := json.NewDecoder(bytes.NewReader(*i))
	if err := dec.Decode(args); err != nil {
		fallbackDec := json.NewDecoder(bytes.NewReader(*i))
		var s *hexutil.Bytes
		if err = fallbackDec.Decode(&s); err != nil {
			return FlipSubmitResponse{}, err
		}
		args.Hex = s
	}
	// temp code end

	if args.Hex == nil {
		return FlipSubmitResponse{}, errors.New("flip is empty")
	}

	rawFlip := *args.Hex

	if len(rawFlip) > MaxFlipSize {
		return FlipSubmitResponse{}, errors.Errorf("flip is too big, max expected size %v, actual %v", MaxFlipSize, len(rawFlip))
	}

	cid, encryptedFlip, err := api.fp.PrepareFlip(rawFlip, args.Pair)

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
		Pair: args.Pair,
	}

	if err := api.fp.AddNewFlip(flip, true); err != nil {
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
	Extra bool   `json:"extra"`
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

	return prepareHashes(api.fp, flips, true)
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

	return prepareHashes(api.fp, flips, false)
}

func prepareHashes(flipper *flip.Flipper, flips [][]byte, shortSession bool) ([]FlipHashesResponse, error) {
	if flips == nil {
		return nil, errors.New("no flips to solve")
	}

	var result []FlipHashesResponse
	for _, v := range flips {
		extraFlip := false
		if shortSession && len(result) >= int(common.ShortSessionFlipsCount()) {
			extraFlip = true
		}
		cid, _ := cid.Parse(v)
		result = append(result, FlipHashesResponse{
			Hash:  cid.String(),
			Ready: flipper.IsFlipReady(v),
			Extra: extraFlip,
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
	Hash   string       `json:"hash"`
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

	flips := api.ceremony.GetShortFlipsToSolve()

	answers := prepareAnswers(args.Answers, flips)

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

	flips := api.ceremony.GetLongFlipsToSolve()

	answers := prepareAnswers(args.Answers, flips)

	hash, err := api.ceremony.SubmitLongAnswers(answers)

	if err != nil {
		return SubmitAnswersResponse{}, err
	}

	return SubmitAnswersResponse{
		TxHash: hash,
	}, nil
}

func prepareAnswers(answers []FlipAnswer, flips [][]byte) *types.Answers {
	findAnswer := func(hash []byte) *FlipAnswer {
		for _, h := range answers {
			c, err := cid.Parse(h.Hash)
			if err == nil && bytes.Compare(c.Bytes(), hash) == 0 {
				return &h
			}
		}
		return nil
	}

	result := types.NewAnswers(uint(len(flips)))

	for i, flip := range flips {
		answer := findAnswer(flip)
		if answer == nil {
			continue
		}
		switch answer.Answer {
		case types.None:
			continue
		case types.Left:
			result.Left(uint(i))
		case types.Right:
			result.Right(uint(i))
		case types.Inappropriate:
			result.Inappropriate(uint(i))
		}
		if answer.Easy {
			result.Easy(uint(i))
		}
	}

	return result
}
