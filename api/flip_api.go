package api

import (
	"bytes"
	"context"
	mapset "github.com/deckarep/golang-set"
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

type RawFlipSubmitArgs struct {
	Tx                  *hexutil.Bytes `json:"tx"`
	EncryptedPublicHex  *hexutil.Bytes `json:"encryptedPublicHex"`
	EncryptedPrivateHex *hexutil.Bytes `json:"encryptedPrivateHex"`
}

func (api *FlipApi) RawSubmit(args RawFlipSubmitArgs) (FlipSubmitResponse, error) {
	if args.EncryptedPublicHex == nil || args.EncryptedPrivateHex == nil || args.Tx == nil {
		return FlipSubmitResponse{}, errors.New("all fields are required")
	}

	encPublicPart := *args.EncryptedPublicHex
	encPrivatePart := *args.EncryptedPrivateHex

	tx := new(types.Transaction)
	err := tx.FromBytes(*args.Tx)
	if err != nil {
		return FlipSubmitResponse{}, err
	}

	flip := &types.Flip{
		Tx:          tx,
		PublicPart:  encPublicPart,
		PrivatePart: encPrivatePart,
	}

	if err := api.fp.AddNewFlip(flip, true); err != nil {
		return FlipSubmitResponse{}, err
	}

	sender, _ := types.Sender(tx)
	log.Info("Raw flip submitted", "hash", tx.Hash().Hex(), "sender", sender.Hex())

	return FlipSubmitResponse{
		TxHash: tx.Hash(),
	}, nil
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
	Hash      string `json:"hash"`
	Ready     bool   `json:"ready"`
	Extra     bool   `json:"extra"`
	Available bool   `json:"available"`
}

func (api *FlipApi) isCeremonyCandidate(addr common.Address) bool {
	identity := api.baseApi.getReadonlyAppState().State.GetIdentity(addr)
	return state.IsCeremonyCandidate(identity)
}

func (api *FlipApi) ShortHashes(addr *common.Address) ([]FlipHashesResponse, error) {
	log.Info("short hashes request")
	defer log.Info("short hashes response")

	coinbase := api.baseApi.getCurrentCoinbase()

	var address common.Address
	if addr != nil {
		address = *addr
	} else {
		address = coinbase
	}
	appState := api.baseApi.getReadonlyAppState()
	period := appState.State.ValidationPeriod()

	if period != state.FlipLotteryPeriod && period != state.ShortSessionPeriod {
		return nil, errors.New("this method is available during FlipLottery and ShortSession periods")
	}

	if !api.isCeremonyCandidate(address) {
		return nil, errors.Errorf("0x%x address is not a ceremony candidate", address)
	}

	flips := api.ceremony.GetShortFlipsToSolve(address, appState.State.ShardId(address))

	return prepareHashes(api.ceremony, flips, true, address == coinbase)
}

func (api *FlipApi) LongHashes(addr *common.Address) ([]FlipHashesResponse, error) {
	log.Info("long hashes request")
	defer log.Info("long hashes response")

	coinbase := api.baseApi.getCurrentCoinbase()

	var address common.Address
	if addr != nil {
		address = *addr
	} else {
		address = coinbase
	}
	appState := api.baseApi.getReadonlyAppState()
	period := appState.State.ValidationPeriod()

	if period != state.FlipLotteryPeriod && period != state.ShortSessionPeriod && period != state.LongSessionPeriod {
		return nil, errors.New("this method is available during FlipLottery, ShortSession and LongSession periods")
	}

	if !api.isCeremonyCandidate(address) {
		return nil, errors.Errorf("0x%x address is not a ceremony candidate", address)
	}

	flips := api.ceremony.GetLongFlipsToSolve(address, appState.State.ShardId(address))

	return prepareHashes(api.ceremony, flips, false, address == coinbase)
}

func prepareHashes(ceremony *ceremony.ValidationCeremony, flips [][]byte, shortSession bool, isCoinbaseAddress bool) ([]FlipHashesResponse, error) {
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
		h := FlipHashesResponse{
			Hash:  cid.String(),
			Extra: extraFlip,
		}

		if isCoinbaseAddress {
			h.Ready = ceremony.IsFlipReadyToSolve(v)
			h.Available = ceremony.IsFlipInMemory(v)
		}
		result = append(result, h)
	}

	return result, nil
}

type FlipResponse struct {
	Hex        hexutil.Bytes `json:"hex"`
	PrivateHex hexutil.Bytes `json:"privateHex"`
}

// Works only for coinbase address
func (api *FlipApi) Get(hash string) (FlipResponse, error) {
	log.Info("get flip request", "hash", hash)
	defer log.Info("get flip response", "hash", hash)

	c, err := cid.Decode(hash)
	if err != nil {
		return FlipResponse{}, err
	}
	cidBytes := c.Bytes()

	publicPart, privatePart, err := api.ceremony.GetDecryptedFlip(cidBytes, api.baseApi.secStore.GetAddress())

	if err != nil {
		return FlipResponse{}, err
	}

	return FlipResponse{
		Hex:        publicPart,
		PrivateHex: privatePart,
	}, nil
}

type FlipResponse2 struct {
	PublicHex  hexutil.Bytes `json:"publicHex"`
	PrivateHex hexutil.Bytes `json:"privateHex"`
}

func (api *FlipApi) GetRaw(hash string) (FlipResponse2, error) {
	log.Info("get raw flip request", "hash", hash)
	defer log.Info("get raw flip response", "hash", hash)

	c, err := cid.Decode(hash)
	if err != nil {
		return FlipResponse2{}, err
	}
	cidBytes := c.Bytes()

	f, err := api.fp.GetRawFlip(cidBytes)

	if err != nil {
		return FlipResponse2{}, err
	}

	return FlipResponse2{
		PublicHex:  f.PublicPart,
		PrivateHex: f.PrivatePart,
	}, nil
}

type FlipKeysResponse struct {
	PublicKey  hexutil.Bytes `json:"publicKey"`
	PrivateKey hexutil.Bytes `json:"privateKey"`
}

func (api *FlipApi) GetKeys(addr common.Address, hash string) (FlipKeysResponse, error) {
	c, err := cid.Decode(hash)
	if err != nil {
		return FlipKeysResponse{}, err
	}
	cidBytes := c.Bytes()
	publicKey, privateKey, err := api.ceremony.GetFlipKeys(addr, cidBytes)

	if err != nil {
		return FlipKeysResponse{}, err
	}

	return FlipKeysResponse{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}, nil
}

type EncryptionKeyArgs struct {
	Data      hexutil.Bytes `json:"data"`
	Signature hexutil.Bytes `json:"signature"`
	Epoch     uint16        `json:"epoch"`
}

func (api *FlipApi) SendPublicEncryptionKey(args EncryptionKeyArgs) error {
	msg := &types.PublicFlipKey{
		Key:       args.Data,
		Epoch:     args.Epoch,
		Signature: args.Signature,
	}
	sender, _ := types.SenderFlipKey(msg)
	log.Info("send public key request", "sender", sender.Hex())
	err := api.ceremony.SendPublicEncryptionKey(msg)
	log.Info("send public key response", "sender", sender.Hex(), "err", err)
	return err
}

func (api *FlipApi) SendPrivateEncryptionKeysPackage(args EncryptionKeyArgs) error {
	msg := &types.PrivateFlipKeysPackage{
		Data:      args.Data,
		Epoch:     args.Epoch,
		Signature: args.Signature,
	}
	sender, _ := types.SenderFlipKeysPackage(msg)
	log.Info("send private package request", "sender", sender.Hex())
	err := api.ceremony.SendPrivateEncryptionKeysPackage(msg)
	log.Info("send private package response", "sender", sender.Hex(), "err", err)
	return err
}

func (api *FlipApi) PrivateEncryptionKeyCandidates(addr common.Address) ([]hexutil.Bytes, error) {
	pubKeys, err := api.ceremony.PrivateEncryptionKeyCandidates(addr)
	if err != nil {
		return nil, err
	}

	var result []hexutil.Bytes

	for _, item := range pubKeys {
		result = append(result, item)
	}

	return result, nil
}

type FlipAnswer struct {
	// Deprecated
	WrongWords *bool        `json:"wrongWords"`
	Grade      types.Grade  `json:"grade"`
	Answer     types.Answer `json:"answer"`
	Hash       string       `json:"hash"`
}

type SubmitAnswersArgs struct {
	Answers []FlipAnswer `json:"answers"`
}

type SubmitAnswersResponse struct {
	TxHash common.Hash `json:"txHash"`
}

func (api *FlipApi) SubmitShortAnswers(args SubmitAnswersArgs) (SubmitAnswersResponse, error) {
	log.Info("short answers submitting request")
	defer log.Info("short answers submitting response")

	if !api.isCeremonyCandidate(api.baseApi.getCurrentCoinbase()) {
		return SubmitAnswersResponse{}, errors.New("coinbase address is not a ceremony candidate")
	}

	flips := api.ceremony.GetShortFlipsToSolve(api.baseApi.getCurrentCoinbase(), api.baseApi.getCoinbaseShard())

	answers := prepareAnswers(args.Answers, flips, true)

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

	if !api.isCeremonyCandidate(api.baseApi.getCurrentCoinbase()) {
		return SubmitAnswersResponse{}, errors.New("coinbase address is not a ceremony candidate")
	}

	flips := api.ceremony.GetLongFlipsToSolve(api.baseApi.getCurrentCoinbase(), api.baseApi.getCoinbaseShard())

	answers := prepareAnswers(args.Answers, flips, false)

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

func prepareAnswers(answers []FlipAnswer, flips [][]byte, isShort bool) *types.Answers {
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
	reportsCount := 0

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
		}
		if isShort {
			continue
		}
		grade := answer.Grade
		if grade == types.GradeNone && answer.WrongWords != nil && *answer.WrongWords {
			grade = types.GradeReported
		}

		// TODO temporary code to support old UI versions: ignore excess reports
		if grade == types.GradeReported {
			reportsCount++
			if float32(reportsCount)/float32(len(flips)) >= 0.34 {
				grade = types.GradeNone
			}
		}

		result.Grade(uint(i), grade)
	}

	return result
}

func (api *FlipApi) WordPairs(addr common.Address, vrfHash hexutil.Bytes) []FlipWords {
	identity := api.baseApi.getReadonlyAppState().State.GetIdentity(addr)
	var hash [32]byte
	copy(hash[:], vrfHash[:])

	firstIndex, wordsDictionarySize := api.ceremony.GetWordDictionaryRange()
	wordPairs := ceremony.GeneratePairsFromVrfHash(hash, firstIndex, wordsDictionarySize, identity.GetTotalWordPairsCount())

	usedPairs := mapset.NewSet()
	for _, v := range identity.Flips {
		usedPairs.Add(v.Pair)
	}

	var convertedFlipKeyWordPairs []FlipWords
	for i := 0; i < len(wordPairs)/2; i++ {
		convertedFlipKeyWordPairs = append(convertedFlipKeyWordPairs,
			FlipWords{
				Words: [2]uint32{uint32(wordPairs[i*2]), uint32(wordPairs[i*2+1])},
				Used:  usedPairs.Contains(uint8(i)),
				Id:    i,
			})
	}

	return convertedFlipKeyWordPairs
}
