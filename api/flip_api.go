package api

import (
	"bytes"
	"context"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/ceremony"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
)

type FlipApi struct {
	baseApi   *BaseApi
	fp        *flip.Flipper
	ipfsProxy ipfs.Proxy
	ceremony  *ceremony.ValidationCeremony
}

// NewFlipApi creates a new FlipApi instance
func NewFlipApi(baseApi *BaseApi, fp *flip.Flipper, ipfsProxy ipfs.Proxy, ceremony *ceremony.ValidationCeremony) *FlipApi {
	return &FlipApi{baseApi, fp, ipfsProxy, ceremony}
}

type FlipSubmitResponse struct {
	TxHash common.Hash `json:"txHash"`
	Hash   string      `json:"hash"`
}

type FlipSubmitArgs struct {
	Hex        *hexutil.Bytes `json:"hex"`
	PublicHex  *hexutil.Bytes `json:"publicHex"`
	PrivateHex *hexutil.Bytes `json:"privateHex"`
	PairId     uint8          `json:"pairId"`
}

func (api *FlipApi) Submit(args FlipSubmitArgs) (FlipSubmitResponse, error) {
	if args.Hex == nil && args.PublicHex == nil {
		return FlipSubmitResponse{}, errors.New("flip is empty")
	}

	var rawPublicPart, rawPrivatePart []byte
	if args.PublicHex != nil {
		rawPublicPart = *args.PublicHex
	} else {
		rawPublicPart = *args.Hex
	}
	if args.PrivateHex != nil {
		rawPrivatePart = *args.PrivateHex
	}

	cid, encryptedPublicPart, encryptedPrivatePart, err := api.fp.PrepareFlip(rawPublicPart, rawPrivatePart)

	if err != nil {
		return FlipSubmitResponse{}, err
	}

	addr := api.baseApi.getCurrentCoinbase()

	tx, err := api.baseApi.getSignedTx(addr, nil, types.SubmitFlipTx, decimal.Zero, decimal.Zero, decimal.Zero, 0, 0, attachments.CreateFlipSubmitAttachment(cid.Bytes(), args.PairId), nil)

	log.Info("Building new flip tx", "hash", tx.Hash().Hex(), "nonce", tx.AccountNonce, "epoch", tx.Epoch)

	if err != nil {
		return FlipSubmitResponse{}, err
	}

	flip := &types.Flip{
		Tx:          tx,
		PublicPart:  encryptedPublicPart,
		PrivatePart: encryptedPrivatePart,
	}

	if err := api.fp.AddNewFlip(flip, true); err != nil {
		return FlipSubmitResponse{}, err
	}

	log.Info("Flip submitted", "hash", tx.Hash().Hex())

	return FlipSubmitResponse{
		TxHash: tx.Hash(),
		Hash:   cid.String(),
	}, nil
}

func (api *FlipApi) Delete(ctx context.Context, hash string) (common.Hash, error) {
	c, err := cid.Decode(hash)
	if err != nil {
		return common.Hash{}, errors.New("invalid flip hash")
	}
	cidBytes := c.Bytes()
	addr := api.baseApi.getCurrentCoinbase()

	if txHash, err := api.baseApi.sendTx(ctx, addr, nil, types.DeleteFlipTx, decimal.Zero, decimal.Zero, decimal.Zero,
		0, 0, attachments.CreateDeleteFlipAttachment(cidBytes), nil); err != nil {
		return common.Hash{}, err
	} else {
		return txHash, nil
	}
}

type FlipHashesResponse struct {
	Hash  string `json:"hash"`
	Ready bool   `json:"ready"`
	Extra bool   `json:"extra"`
}

func (api *FlipApi) isCeremonyCandidate() bool {
	identity := api.baseApi.getAppState().State.GetIdentity(api.baseApi.getCurrentCoinbase())
	return state.IsCeremonyCandidate(identity)
}

func (api *FlipApi) ShortHashes() ([]FlipHashesResponse, error) {
	log.Info("short hashes request")
	defer log.Info("short hashes response")

	period := api.baseApi.getAppState().State.ValidationPeriod()

	if period != state.FlipLotteryPeriod && period != state.ShortSessionPeriod {
		return nil, errors.New("this method is available during FlipLottery and ShortSession periods")
	}

	if !api.isCeremonyCandidate() {
		return nil, errors.New("coinbase address is not a ceremony candidate")
	}

	flips := api.ceremony.GetShortFlipsToSolve()

	return prepareHashes(api.fp, flips, true)
}

func (api *FlipApi) LongHashes() ([]FlipHashesResponse, error) {
	log.Info("long hashes request")
	defer log.Info("long hashes response")

	period := api.baseApi.getAppState().State.ValidationPeriod()

	if period != state.FlipLotteryPeriod && period != state.ShortSessionPeriod && period != state.LongSessionPeriod {
		return nil, errors.New("this method is available during FlipLottery, ShortSession and LongSession periods")
	}

	if !api.isCeremonyCandidate() {
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
	Hex        hexutil.Bytes `json:"hex"`
	PrivateHex hexutil.Bytes `json:"privateHex"`
}

func (api *FlipApi) Get(hash string) (FlipResponse, error) {
	log.Info("get flip request", "hash", hash)
	defer log.Info("get flip response", "hash", hash)

	c, err := cid.Decode(hash)
	if err != nil {
		return FlipResponse{}, err
	}
	cidBytes := c.Bytes()

	publicPart, privatePart, err := api.fp.GetFlip(cidBytes)

	if err != nil {
		return FlipResponse{}, err
	}

	return FlipResponse{
		Hex:        publicPart,
		PrivateHex: privatePart,
	}, nil
}

type FlipAnswer struct {
	WrongWords bool         `json:"wrongWords"`
	Answer     types.Answer `json:"answer"`
	Hash       string       `json:"hash"`
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
	log.Info("short answers submitting request")
	defer log.Info("short answers submitting response")

	if !api.isCeremonyCandidate() {
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
	log.Info("long answers submitting request")
	defer log.Info("long answers submitting response")

	if !api.isCeremonyCandidate() {
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

type FlipWordsResponse struct {
	Words [2]int `json:"words"`
}

func (api *FlipApi) Words(hash string) (FlipWordsResponse, error) {
	c, err := cid.Parse(hash)
	if err != nil {
		return FlipWordsResponse{}, err
	}
	w1, w2, err := api.ceremony.GetFlipWords(c.Bytes())
	return FlipWordsResponse{
		Words: [2]int{w1, w2},
	}, err
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
		if answer.WrongWords {
			result.WrongWords(uint(i))
		}
	}

	return result
}
