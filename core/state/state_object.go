package state

import (
	"bytes"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	models "github.com/idena-network/idena-go/protobuf"
	math2 "math"
	"math/big"
)

type IdentityState uint8

const (
	Undefined IdentityState = 0
	Invite    IdentityState = 1
	Candidate IdentityState = 2
	Verified  IdentityState = 3
	Suspended IdentityState = 4
	Killed    IdentityState = 5
	Zombie    IdentityState = 6
	Newbie    IdentityState = 7
	Human     IdentityState = 8

	MaxInvitesAmount    = math.MaxUint8
	EmptyBlocksBitsSize = 25

	AdditionalVerifiedFlips = 1
	AdditionalHumanFlips    = 2

	// Minimal number of blocks in after long period which should be without ceremonial txs
	AfterLongRequiredBlocks = 5
)

func (s IdentityState) NewbieOrBetter() bool {
	return s == Newbie || s == Verified || s == Human
}

func (s IdentityState) VerifiedOrBetter() bool {
	return s == Verified || s == Human
}

// stateAccount represents an Idena account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type stateAccount struct {
	address common.Address
	data    Account

	deleted bool
	onDirty func(addr common.Address) // Callback method to mark a state object newly dirty
}

type stateIdentity struct {
	address common.Address
	data    Identity

	deleted bool
	onDirty func(addr common.Address) // Callback method to mark a state object newly dirty
}

type stateApprovedIdentity struct {
	address common.Address
	data    ApprovedIdentity

	deleted bool
	onDirty func(addr common.Address) // Callback method to mark a state object newly dirty
}

type stateGlobal struct {
	data Global

	onDirty func() // Callback method to mark a state object newly dirty
}

type stateStatusSwitch struct {
	data IdentityStatusSwitch

	deleted bool
	onDirty func()
}

type ValidationPeriod uint32

const (
	NonePeriod             ValidationPeriod = 0
	FlipLotteryPeriod      ValidationPeriod = 1
	ShortSessionPeriod     ValidationPeriod = 2
	LongSessionPeriod      ValidationPeriod = 3
	AfterLongSessionPeriod ValidationPeriod = 4
)

type IdentityStatusSwitch struct {
	Addresses []common.Address `rlp:"nil"`
}

func (s *IdentityStatusSwitch) ToBytes() ([]byte, error) {
	protoObj := new(models.ProtoStateIdentityStatusSwitch)
	for idx := range s.Addresses {
		protoObj.Addresses = append(protoObj.Addresses, s.Addresses[idx][:])
	}
	return proto.Marshal(protoObj)
}

func (s *IdentityStatusSwitch) FromBytes(data []byte) error {
	protoObj := new(models.ProtoStateIdentityStatusSwitch)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	for idx := range protoObj.Addresses {
		s.Addresses = append(s.Addresses, common.BytesToAddress(protoObj.Addresses[idx]))
	}
	return nil
}

type Global struct {
	Epoch                         uint16
	NextValidationTime            int64
	ValidationPeriod              ValidationPeriod
	GodAddress                    common.Address
	WordsSeed                     types.Seed `rlp:"nil"`
	LastSnapshot                  uint64
	EpochBlock                    uint64
	FeePerByte                    *big.Int
	VrfProposerThreshold          uint64
	EmptyBlocksBits               *big.Int
	GodAddressInvites             uint16
	BlocksCntWithoutCeremonialTxs byte
}

func (s *Global) ToBytes() ([]byte, error) {
	protoAnswer := &models.ProtoStateGlobal{
		Epoch:                         uint32(s.Epoch),
		NextValidationTime:            s.NextValidationTime,
		ValidationPeriod:              uint32(s.ValidationPeriod),
		GodAddress:                    s.GodAddress[:],
		WordsSeed:                     s.WordsSeed[:],
		LastSnapshot:                  s.LastSnapshot,
		EpochBlock:                    s.EpochBlock,
		FeePerByte:                    common.BigIntBytesOrNil(s.FeePerByte),
		VrfProposerThreshold:          s.VrfProposerThreshold,
		EmptyBlocksBits:               common.BigIntBytesOrNil(s.EmptyBlocksBits),
		GodAddressInvites:             uint32(s.GodAddressInvites),
		BlocksCntWithoutCeremonialTxs: uint32(s.BlocksCntWithoutCeremonialTxs),
	}
	return proto.Marshal(protoAnswer)
}

func (s *Global) FromBytes(data []byte) error {
	protoGlobal := new(models.ProtoStateGlobal)
	if err := proto.Unmarshal(data, protoGlobal); err != nil {
		return err
	}

	s.Epoch = uint16(protoGlobal.Epoch)
	s.NextValidationTime = protoGlobal.NextValidationTime
	s.ValidationPeriod = ValidationPeriod(protoGlobal.ValidationPeriod)
	s.GodAddress = common.BytesToAddress(protoGlobal.GodAddress)
	s.WordsSeed = types.BytesToSeed(protoGlobal.WordsSeed)
	s.LastSnapshot = protoGlobal.LastSnapshot
	s.EpochBlock = protoGlobal.EpochBlock
	s.FeePerByte = common.BigIntOrNil(protoGlobal.FeePerByte)
	s.VrfProposerThreshold = protoGlobal.VrfProposerThreshold
	s.EmptyBlocksBits = common.BigIntOrNil(protoGlobal.EmptyBlocksBits)
	s.GodAddressInvites = uint16(protoGlobal.GodAddressInvites)
	s.BlocksCntWithoutCeremonialTxs = byte(protoGlobal.BlocksCntWithoutCeremonialTxs)
	return nil
}

// Account is the Idena consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint32
	Epoch    uint16
	Balance  *big.Int
	Contract *ContractData
}

type ContractData struct {
	CodeHash common.Hash
	Stake    *big.Int
}

func (a *Account) ToBytes() ([]byte, error) {
	protoAcc := &models.ProtoStateAccount{
		Nonce:   a.Nonce,
		Epoch:   uint32(a.Epoch),
		Balance: common.BigIntBytesOrNil(a.Balance),
	}
	if a.Contract != nil {
		protoAcc.ContractData = &models.ProtoContractData{
			CodeHash: a.Contract.CodeHash.Bytes(),
			Stake:    common.BigIntBytesOrNil(a.Contract.Stake),
		}
	}
	return proto.Marshal(protoAcc)
}

func (a *Account) FromBytes(data []byte) error {
	protoAcc := new(models.ProtoStateAccount)
	if err := proto.Unmarshal(data, protoAcc); err != nil {
		return err
	}
	a.Balance = common.BigIntOrNil(protoAcc.Balance)
	a.Epoch = uint16(protoAcc.Epoch)
	a.Nonce = protoAcc.Nonce
	if protoAcc.ContractData != nil {
		hash := common.Hash{}
		hash.SetBytes(protoAcc.ContractData.CodeHash)
		a.Contract = &ContractData{
			CodeHash: hash,
			Stake:    common.BigIntOrNil(protoAcc.ContractData.Stake),
		}
	}
	return nil
}

type IdentityFlip struct {
	Cid  []byte
	Pair uint8
}

type ValidationStatusFlag uint16

const (
	AtLeastOneFlipReported ValidationStatusFlag = 1 << iota
	AtLeastOneFlipNotQualified
	AllFlipsNotQualified
)

type Identity struct {
	ProfileHash    []byte
	Stake          *big.Int
	Invites        uint8
	Birthday       uint16
	State          IdentityState
	QualifiedFlips uint32
	// should use GetShortFlipPoints instead of reading directly
	ShortFlipPoints      uint32
	PubKey               []byte
	RequiredFlips        uint8
	Flips                []IdentityFlip
	Generation           uint32
	Code                 []byte
	Invitees             []TxAddr
	Inviter              *TxAddr
	Penalty              *big.Int
	ValidationTxsBits    byte
	LastValidationStatus ValidationStatusFlag
	Scores               []byte
}

type TxAddr struct {
	TxHash  common.Hash
	Address common.Address
}

func (i *Identity) ToBytes() ([]byte, error) {
	protoIdentity := &models.ProtoStateIdentity{
		Stake:            common.BigIntBytesOrNil(i.Stake),
		Invites:          uint32(i.Invites),
		Birthday:         uint32(i.Birthday),
		State:            uint32(i.State),
		QualifiedFlips:   i.QualifiedFlips,
		ShortFlipPoints:  i.ShortFlipPoints,
		PubKey:           i.PubKey,
		RequiredFlips:    uint32(i.RequiredFlips),
		Generation:       i.Generation,
		Code:             i.Code,
		Penalty:          common.BigIntBytesOrNil(i.Penalty),
		ValidationBits:   uint32(i.ValidationTxsBits),
		ValidationStatus: uint32(i.LastValidationStatus),
		ProfileHash:      i.ProfileHash,
		Scores:           i.Scores,
	}
	for idx := range i.Flips {
		protoIdentity.Flips = append(protoIdentity.Flips, &models.ProtoStateIdentity_Flip{
			Cid:  i.Flips[idx].Cid,
			Pair: uint32(i.Flips[idx].Pair),
		})
	}
	for idx := range i.Invitees {
		protoIdentity.Invitees = append(protoIdentity.Invitees, &models.ProtoStateIdentity_TxAddr{
			Hash:    i.Invitees[idx].TxHash[:],
			Address: i.Invitees[idx].Address[:],
		})
	}
	if i.Inviter != nil {
		protoIdentity.Inviter = &models.ProtoStateIdentity_TxAddr{
			Hash:    i.Inviter.TxHash[:],
			Address: i.Inviter.Address[:],
		}
	}
	return proto.Marshal(protoIdentity)
}

func (i *Identity) FromBytes(data []byte) error {
	protoIdentity := new(models.ProtoStateIdentity)
	if err := proto.Unmarshal(data, protoIdentity); err != nil {
		return err
	}
	i.Stake = common.BigIntOrNil(protoIdentity.Stake)
	i.Invites = uint8(protoIdentity.Invites)
	i.Birthday = uint16(protoIdentity.Birthday)
	i.State = IdentityState(protoIdentity.State)
	i.QualifiedFlips = protoIdentity.QualifiedFlips
	i.ShortFlipPoints = protoIdentity.ShortFlipPoints
	i.PubKey = protoIdentity.PubKey
	i.RequiredFlips = uint8(protoIdentity.RequiredFlips)
	i.Generation = protoIdentity.Generation
	i.Code = protoIdentity.Code
	i.Penalty = common.BigIntOrNil(protoIdentity.Penalty)
	i.ValidationTxsBits = byte(protoIdentity.ValidationBits)
	i.LastValidationStatus = ValidationStatusFlag(protoIdentity.ValidationStatus)
	i.ProfileHash = protoIdentity.ProfileHash
	i.Scores = protoIdentity.Scores

	for idx := range protoIdentity.Flips {
		i.Flips = append(i.Flips, IdentityFlip{
			Cid:  protoIdentity.Flips[idx].Cid,
			Pair: uint8(protoIdentity.Flips[idx].Pair),
		})
	}

	for idx := range protoIdentity.Invitees {
		i.Invitees = append(i.Invitees, TxAddr{
			TxHash:  common.BytesToHash(protoIdentity.Invitees[idx].Hash),
			Address: common.BytesToAddress(protoIdentity.Invitees[idx].Address),
		})
	}

	if protoIdentity.Inviter != nil {
		i.Inviter = &TxAddr{
			TxHash:  common.BytesToHash(protoIdentity.Inviter.Hash),
			Address: common.BytesToAddress(protoIdentity.Inviter.Address),
		}
	}

	return nil
}

func (i *Identity) GetShortFlipPoints() float32 {
	return float32(i.ShortFlipPoints) / 2
}

func (i *Identity) HasDoneAllRequiredFlips() bool {
	return uint8(len(i.Flips)) >= i.RequiredFlips
}

func (i *Identity) GetTotalWordPairsCount() int {
	return common.WordPairsPerFlip * int(i.RequiredFlips)
}

func (i *Identity) GetMaximumAvailableFlips() uint8 {
	if i.State == Verified {
		return i.RequiredFlips + AdditionalVerifiedFlips
	}
	if i.State == Human {
		return i.RequiredFlips + AdditionalHumanFlips
	}
	return i.RequiredFlips
}

type ApprovedIdentity struct {
	Approved bool
	Online   bool
}

func (s *ApprovedIdentity) ToBytes() ([]byte, error) {
	protoAnswer := &models.ProtoStateApprovedIdentity{
		Approved: s.Approved,
		Online:   s.Online,
	}
	return proto.Marshal(protoAnswer)
}

func (s *ApprovedIdentity) FromBytes(data []byte) error {
	protoIdentity := new(models.ProtoStateApprovedIdentity)
	if err := proto.Unmarshal(data, protoIdentity); err != nil {
		return err
	}
	s.Approved = protoIdentity.Approved
	s.Online = protoIdentity.Online
	return nil
}

// newAccountObject creates a state object.
func newAccountObject(address common.Address, data Account, onDirty func(addr common.Address)) *stateAccount {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}

	return &stateAccount{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

// newAccountObject creates a state object.
func newIdentityObject(address common.Address, data Identity, onDirty func(addr common.Address)) *stateIdentity {

	return &stateIdentity{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

// newGlobalObject creates a global state object.
func newGlobalObject(data Global, onDirty func()) *stateGlobal {
	return &stateGlobal{
		data:    data,
		onDirty: onDirty,
	}
}

func newStatusSwitchObject(data IdentityStatusSwitch, onDirty func()) *stateStatusSwitch {
	return &stateStatusSwitch{
		data:    data,
		onDirty: onDirty,
	}
}

func newApprovedIdentityObject(address common.Address, data ApprovedIdentity, onDirty func(addr common.Address)) *stateApprovedIdentity {
	return &stateApprovedIdentity{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

func (s *stateAccount) touch() {
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (s *stateAccount) AddBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}

		return
	}
	s.SetBalance(new(big.Int).Add(s.Balance(), amount))
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (s *stateAccount) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	s.SetBalance(new(big.Int).Sub(s.Balance(), amount))
}

func (s *stateAccount) SetBalance(amount *big.Int) {
	s.setBalance(amount)
}

func (s *stateAccount) setBalance(amount *big.Int) {
	s.data.Balance = amount
	s.touch()
}

func (s *stateAccount) deepCopy(db *StateDB, onDirty func(addr common.Address)) *stateAccount {
	stateObject := newAccountObject(s.address, s.data, onDirty)
	stateObject.deleted = s.deleted
	return stateObject
}

// empty returns whether the account is considered empty.
func (s *stateAccount) empty() bool {
	return s.Balance().Sign() == 0 && s.data.Nonce == 0 && s.data.Contract == nil
}

// Returns the address of the contract/account
func (s *stateAccount) Address() common.Address {
	return s.address
}

func (s *stateAccount) SetNonce(nonce uint32) {
	s.setNonce(nonce)
}

func (s *stateAccount) setNonce(nonce uint32) {
	s.data.Nonce = nonce
	s.touch()
}

func (s *stateAccount) SetEpoch(nonce uint16) {
	s.setEpoch(nonce)
}

func (s *stateAccount) setEpoch(nonce uint16) {
	s.data.Epoch = nonce
	s.touch()
}

func (s *stateAccount) Balance() *big.Int {
	if s.data.Balance == nil {
		return big.NewInt(0)
	}
	return s.data.Balance
}

func (s *stateAccount) Nonce() uint32 {
	return s.data.Nonce
}

func (s *stateAccount) Epoch() uint16 {
	return s.data.Epoch
}

func (s *stateAccount) SetCodeHash(hash common.Hash) {
	if s.data.Contract == nil {
		s.data.Contract = &ContractData{}
	}
	s.data.Contract.CodeHash = hash
	s.touch()
}

func (s *stateAccount) SetContractStake(stake *big.Int) {
	if s.data.Contract == nil {
		s.data.Contract = &ContractData{}
	}
	s.data.Contract.Stake = stake
	s.touch()
}

// Returns the address of the contract/account
func (s *stateIdentity) Address() common.Address {
	return s.address
}

func (s *stateIdentity) Invites() uint8 {
	return s.data.Invites
}

func (s *stateIdentity) SetInvites(invites uint8) {
	s.setInvites(invites)
}

func (s *stateIdentity) setInvites(invites uint8) {
	s.data.Invites = invites
	s.touch()
}

// empty returns whether the account is considered empty.
func (s *stateIdentity) empty() bool {
	return s.data.State == Killed
}

func (s *stateIdentity) touch() {
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

func (s *stateIdentity) State() IdentityState {
	return s.data.State
}

func (s *stateIdentity) SetState(state IdentityState) {
	s.data.State = state
	s.touch()
}

func (s *stateIdentity) SetGeneticCode(generation uint32, code []byte) {
	s.data.Generation = generation
	s.data.Code = code
	s.touch()
}

func (s *stateIdentity) GeneticCode() (generation uint32, code []byte) {
	return s.data.Generation, s.data.Code
}

func (s *stateIdentity) Stake() *big.Int {
	if s.data.Stake == nil {
		return big.NewInt(0)
	}
	return s.data.Stake
}

func (s *stateIdentity) AddStake(amount *big.Int) {
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}
		return
	}
	s.SetStake(new(big.Int).Add(s.Stake(), amount))
}

func (s *stateIdentity) SubStake(amount *big.Int) {
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}
		return
	}

	s.SetStake(new(big.Int).Sub(s.Stake(), amount))
}

func (s *stateIdentity) SetStake(amount *big.Int) {
	s.data.Stake = amount
	s.touch()
}

func (s *stateIdentity) AddInvite(i uint8) {
	if s.Invites() == MaxInvitesAmount {
		return
	}
	s.SetInvites(s.Invites() + i)
}

func (s *stateIdentity) SubInvite(i uint8) {
	s.SetInvites(s.Invites() - i)
}

func (s *stateIdentity) SetPubKey(pubKey []byte) {
	s.data.PubKey = pubKey
	s.touch()
}

func (s *stateIdentity) QualifiedFlipsCount() uint32 {
	return s.data.QualifiedFlips
}

func (s *stateIdentity) AddNewScore(score byte) {
	if len(s.data.Scores) == common.LastScoresCount {
		s.data.Scores = append(s.data.Scores[1:], score)
	} else {
		s.data.Scores = append(s.data.Scores, score)
	}

	s.touch()
}

func (s *stateIdentity) Scores() []byte {
	return s.data.Scores
}

func (s *stateIdentity) ShortFlipPoints() float32 {
	return s.data.GetShortFlipPoints()
}

func (s *stateIdentity) GetRequiredFlips() uint8 {
	return s.data.RequiredFlips
}

func (s *stateIdentity) SetRequiredFlips(amount uint8) {
	s.data.RequiredFlips = amount
	s.touch()
}

func (s *stateIdentity) GetMadeFlips() uint8 {
	return uint8(len(s.data.Flips))
}

func (s *stateIdentity) AddFlip(cid []byte, pair uint8) {
	if len(s.data.Flips) < math.MaxUint8 {
		s.data.Flips = append(s.data.Flips, IdentityFlip{
			Cid:  cid,
			Pair: pair,
		})
		s.touch()
	}
}

func (s *stateIdentity) DeleteFlip(cid []byte) {
	for i, flip := range s.data.Flips {
		if bytes.Compare(flip.Cid, cid) != 0 {
			continue
		}
		s.data.Flips = append(s.data.Flips[:i], s.data.Flips[i+1:]...)
		s.touch()
		return
	}
}

func (s *stateIdentity) ClearFlips() {
	s.data.Flips = nil
	s.touch()
}

func (s *stateIdentity) SetInviter(address common.Address, txHash common.Hash) {
	s.data.Inviter = &TxAddr{
		TxHash:  txHash,
		Address: address,
	}
	s.touch()
}

func (s *stateIdentity) GetInviter() *TxAddr {
	return s.data.Inviter
}

func (s *stateIdentity) ResetInviter() {
	s.data.Inviter = nil
	s.touch()
}

func (s *stateIdentity) AddInvitee(address common.Address, txHash common.Hash) {
	s.data.Invitees = append(s.data.Invitees, TxAddr{
		TxHash:  txHash,
		Address: address,
	})
	s.touch()
}

func (s *stateIdentity) GetInvitees() []TxAddr {
	return s.data.Invitees
}

func (s *stateIdentity) RemoveInvitee(address common.Address) {
	if len(s.data.Invitees) == 0 {
		return
	}
	for i, invitee := range s.data.Invitees {
		if invitee.Address != address {
			continue
		}
		s.data.Invitees = append(s.data.Invitees[:i], s.data.Invitees[i+1:]...)
		s.touch()
		return
	}
}

func (s *stateIdentity) SetBirthday(birthday uint16) {
	s.data.Birthday = birthday
	s.touch()
}

func (s *stateIdentity) SetPenalty(penalty *big.Int) {
	s.data.Penalty = penalty
	s.touch()
}

func (s *stateIdentity) SubPenalty(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	p := s.GetPenalty()

	if p.Cmp(amount) <= 0 {
		s.SetPenalty(nil)
	} else {
		s.SetPenalty(new(big.Int).Sub(p, amount))
	}
}

func (s *stateIdentity) GetPenalty() *big.Int {
	if s.data.Penalty == nil {
		return new(big.Int)
	}
	return s.data.Penalty
}

func (s *stateIdentity) SetProfileHash(hash []byte) {
	s.data.ProfileHash = hash
	s.touch()
}

func (s *stateIdentity) GetProfileHash() []byte {
	return s.data.ProfileHash
}

func (s *stateIdentity) SetValidationTxBit(txType types.TxType) {
	mask := validationTxBitMask(txType)
	if mask == 0 {
		return
	}
	s.data.ValidationTxsBits = s.data.ValidationTxsBits | mask
	s.touch()
}

func (s *stateIdentity) HasValidationTx(txType types.TxType) bool {
	mask := validationTxBitMask(txType)
	if mask == 0 {
		return false
	}
	return s.data.ValidationTxsBits&mask > 0
}

func (s *stateIdentity) ResetValidationTxBits() {
	s.data.ValidationTxsBits = 0
	s.touch()
}

func (s *stateIdentity) SetValidationStatus(status ValidationStatusFlag) {
	s.data.LastValidationStatus = status
	s.touch()
}

func (s *stateGlobal) Epoch() uint16 {
	return s.data.Epoch
}

func (s *stateGlobal) IncEpoch() {
	s.data.Epoch++
	s.touch()
}

func (s *stateGlobal) VrfProposerThreshold() float64 {
	return math2.Float64frombits(s.data.VrfProposerThreshold)
}

func (s *stateGlobal) VrfProposerThresholdRaw() uint64 {
	return s.data.VrfProposerThreshold
}

func (s *stateGlobal) SetVrfProposerThreshold(value float64) {
	s.data.VrfProposerThreshold = math2.Float64bits(value)
	s.touch()
}

func (s *stateGlobal) AddBlockBit(empty bool) {
	if s.data.EmptyBlocksBits == nil {
		s.data.EmptyBlocksBits = new(big.Int)
	}
	s.data.EmptyBlocksBits.Lsh(s.data.EmptyBlocksBits, 1)
	if !empty {
		s.data.EmptyBlocksBits.SetBit(s.data.EmptyBlocksBits, 0, 1)
	}
	s.data.EmptyBlocksBits.SetBit(s.data.EmptyBlocksBits, EmptyBlocksBitsSize, 0)
	s.touch()
}

func (s *stateGlobal) EmptyBlocksCount() int {
	bits := s.EmptyBlocksBits()
	cnt := 0
	for i := 0; i < EmptyBlocksBitsSize; i++ {
		if bits.Bit(i) == 0 {
			cnt++
		}
	}
	return cnt
}

func (s *stateGlobal) EmptyBlocksBits() *big.Int {
	if s.data.EmptyBlocksBits == nil {
		return new(big.Int)
	}
	return s.data.EmptyBlocksBits
}

func (s *stateGlobal) LastSnapshot() uint64 {
	return s.data.LastSnapshot
}

func (s *stateGlobal) SetLastSnapshot(height uint64) {
	s.data.LastSnapshot = height
	s.touch()
}

func (s *stateGlobal) ValidationPeriod() ValidationPeriod {
	return s.data.ValidationPeriod
}

func (s *stateGlobal) touch() {
	if s.onDirty != nil {
		s.onDirty()
	}
}

func (s *stateGlobal) NextValidationTime() int64 {
	return s.data.NextValidationTime
}

func (s *stateGlobal) SetNextValidationTime(unix int64) {
	s.data.NextValidationTime = unix
	s.touch()
}

func (s *stateGlobal) SetValidationPeriod(period ValidationPeriod) {
	s.data.ValidationPeriod = period
	s.touch()
}

func (s *stateGlobal) SetGodAddress(godAddress common.Address) {
	s.data.GodAddress = godAddress
	s.touch()
}

func (s *stateGlobal) GodAddress() common.Address {
	return s.data.GodAddress
}

func (s *stateGlobal) SetFlipWordsSeed(seed types.Seed) {
	s.data.WordsSeed = seed
	s.touch()
}

func (s *stateGlobal) FlipWordsSeed() types.Seed {
	return s.data.WordsSeed
}

func (s *stateGlobal) SetEpoch(epoch uint16) {
	s.data.Epoch = epoch
	s.touch()
}

func (s *stateGlobal) SetEpochBlock(height uint64) {
	s.data.EpochBlock = height
	s.touch()
}

func (s *stateGlobal) EpochBlock() uint64 {
	return s.data.EpochBlock
}

func (s *stateGlobal) SetFeePerByte(fee *big.Int) {
	s.data.FeePerByte = fee
	s.touch()
}

func (s *stateGlobal) FeePerByte() *big.Int {
	if s.data.FeePerByte == nil {
		return new(big.Int)
	}
	return s.data.FeePerByte
}

func (s *stateGlobal) SubGodAddressInvite() {
	s.data.GodAddressInvites -= 1
	s.touch()
}

func (s *stateGlobal) GodAddressInvites() uint16 {
	return s.data.GodAddressInvites
}

func (s *stateGlobal) SetGodAddressInvites(count uint16) {
	s.data.GodAddressInvites = count
	s.touch()
}

func (s *stateGlobal) BlocksCntWithoutCeremonialTxs() byte {
	return s.data.BlocksCntWithoutCeremonialTxs
}

func (s *stateGlobal) IncBlocksCntWithoutCeremonialTxs() {
	s.data.BlocksCntWithoutCeremonialTxs++
	s.touch()
}

func (s *stateGlobal) ResetBlocksCntWithoutCeremonialTxs() {
	s.data.BlocksCntWithoutCeremonialTxs = 0
	s.touch()
}

func (s *stateApprovedIdentity) Address() common.Address {
	return s.address
}

func (s *stateApprovedIdentity) Online() bool {
	return s.data.Online
}

// empty returns whether the account is considered empty.
func (s *stateApprovedIdentity) empty() bool {
	return !s.data.Approved
}

func (s *stateApprovedIdentity) touch() {
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

func (s *stateApprovedIdentity) SetState(approved bool) {
	s.data.Approved = approved
	s.touch()
}

func (s *stateApprovedIdentity) SetOnline(online bool) {
	s.data.Online = online
	s.touch()
}

func IsCeremonyCandidate(identity Identity) bool {
	state := identity.State
	return (state == Candidate || state.NewbieOrBetter() || state == Suspended ||
		state == Zombie) && identity.HasDoneAllRequiredFlips()
}

func (s *stateStatusSwitch) empty() bool {
	return len(s.data.Addresses) == 0
}

func (s *stateStatusSwitch) Addresses() []common.Address {
	return s.data.Addresses
}

func (s *stateStatusSwitch) Clear() {
	s.data.Addresses = []common.Address{}
	s.touch()
}

func (s *stateStatusSwitch) ToggleAddress(sender common.Address) {
	defer s.touch()
	for i := 0; i < len(s.data.Addresses); i++ {
		if s.data.Addresses[i] == sender {
			s.data.Addresses = append(s.data.Addresses[:i], s.data.Addresses[i+1:]...)
			return
		}
	}
	s.data.Addresses = append(s.data.Addresses, sender)
}

func (s *stateStatusSwitch) HasAddress(addr common.Address) bool {
	for _, item := range s.data.Addresses {
		if item == addr {
			return true
		}
	}
	return false
}

func (s *stateStatusSwitch) touch() {
	if s.onDirty != nil {
		s.onDirty()
	}
}

func (f ValidationStatusFlag) HasFlag(flag ValidationStatusFlag) bool {
	return f&flag != 0
}
