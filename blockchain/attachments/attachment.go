package attachments

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	models "github.com/idena-network/idena-go/protobuf"
)

type ShortAnswerAttachment struct {
	Answers []byte
	Rnd     uint64
}

func (s *ShortAnswerAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoShortAnswerAttachment{
		Answers: s.Answers,
		Rnd:     s.Rnd,
	}
	return proto.Marshal(protoAttachment)
}

func (s *ShortAnswerAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoShortAnswerAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Answers = protoAttachment.Answers
	s.Rnd = protoAttachment.Rnd
	return nil
}

func CreateShortAnswerAttachment(answers []byte, rnd uint64) []byte {
	attachment := &ShortAnswerAttachment{
		Answers: answers,
		Rnd:     rnd,
	}

	payload, _ := attachment.ToBytes()

	return payload
}

func ParseShortAnswerAttachment(tx *types.Transaction) *ShortAnswerAttachment {
	return ParseShortAnswerBytesAttachment(tx.Payload)
}

func ParseShortAnswerBytesAttachment(payload []byte) *ShortAnswerAttachment {
	if len(payload) == 0 {
		return nil
	}
	attachment := new(ShortAnswerAttachment)
	if err := attachment.FromBytes(payload); err != nil {
		return nil
	}
	return attachment
}

type LongAnswerAttachment struct {
	Answers []byte
	Proof   []byte
	Key     []byte
	Salt    []byte
}

func (s *LongAnswerAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoLongAnswerAttachment{
		Answers: s.Answers,
		Proof:   s.Proof,
		Key:     s.Key,
		Salt:    s.Salt,
	}
	return proto.Marshal(protoAttachment)
}

func (s *LongAnswerAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoLongAnswerAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Answers = protoAttachment.Answers
	s.Proof = protoAttachment.Proof
	s.Key = protoAttachment.Key
	s.Salt = protoAttachment.Salt
	return nil
}

func CreateLongAnswerAttachment(answers []byte, proof []byte, salt []byte, key *ecies.PrivateKey) []byte {
	attachment := &LongAnswerAttachment{
		Answers: answers,
		Proof:   proof,
		Key:     crypto.FromECDSA(key.ExportECDSA()),
		Salt:    salt,
	}

	payload, _ := attachment.ToBytes()

	return payload
}

func ParseLongAnswerAttachment(tx *types.Transaction) *LongAnswerAttachment {
	return ParseLongAnswerBytesAttachment(tx.Payload)
}

func ParseLongAnswerBytesAttachment(payload []byte) *LongAnswerAttachment {
	if len(payload) == 0 {
		return nil
	}
	attachment := new(LongAnswerAttachment)
	if err := attachment.FromBytes(payload); err != nil {
		return nil
	}
	return attachment
}

type FlipSubmitAttachment struct {
	Cid  []byte
	Pair uint8
}

func (s *FlipSubmitAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoFlipSubmitAttachment{
		Cid:  s.Cid,
		Pair: uint32(s.Pair),
	}
	return proto.Marshal(protoAttachment)
}

func (s *FlipSubmitAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoFlipSubmitAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Cid = protoAttachment.Cid
	s.Pair = uint8(protoAttachment.Pair)
	return nil
}

func CreateFlipSubmitAttachment(cid []byte, pair uint8) []byte {
	attachment := &FlipSubmitAttachment{
		Cid:  cid,
		Pair: pair,
	}
	payload, _ := attachment.ToBytes()
	return payload
}

func ParseFlipSubmitAttachment(tx *types.Transaction) *FlipSubmitAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(FlipSubmitAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type OnlineStatusAttachment struct {
	Online bool
}

func (s *OnlineStatusAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoOnlineStatusAttachment{
		Online: s.Online,
	}
	return proto.Marshal(protoAttachment)
}

func (s *OnlineStatusAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoOnlineStatusAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Online = protoAttachment.Online
	return nil
}

func CreateOnlineStatusAttachment(online bool) []byte {
	attachment := &OnlineStatusAttachment{
		Online: online,
	}
	payload, _ := attachment.ToBytes()
	return payload
}

func ParseOnlineStatusAttachment(tx *types.Transaction) *OnlineStatusAttachment {
	attachment := new(OnlineStatusAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type BurnAttachment struct {
	Key string
}

func (s *BurnAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoBurnAttachment{
		Key: s.Key,
	}
	return proto.Marshal(protoAttachment)
}

func (s *BurnAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoBurnAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Key = protoAttachment.Key
	return nil
}

func CreateBurnAttachment(key string) []byte {
	attachment := &BurnAttachment{
		Key: key,
	}
	payload, _ := attachment.ToBytes()
	return payload
}

func ParseBurnAttachment(tx *types.Transaction) *BurnAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(BurnAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type ChangeProfileAttachment struct {
	Hash []byte
}

func (s *ChangeProfileAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoChangeProfileAttachment{
		Hash: s.Hash,
	}
	return proto.Marshal(protoAttachment)
}

func (s *ChangeProfileAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoChangeProfileAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Hash = protoAttachment.Hash
	return nil
}

func CreateChangeProfileAttachment(hash []byte) []byte {
	attachment := &ChangeProfileAttachment{
		Hash: hash,
	}
	payload, _ := attachment.ToBytes()
	return payload
}

func ParseChangeProfileAttachment(tx *types.Transaction) *ChangeProfileAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(ChangeProfileAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type DeleteFlipAttachment struct {
	Cid []byte
}

func (s *DeleteFlipAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoDeleteFlipAttachment{
		Cid: s.Cid,
	}
	return proto.Marshal(protoAttachment)
}

func (s *DeleteFlipAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoDeleteFlipAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	s.Cid = protoAttachment.Cid
	return nil
}

func CreateDeleteFlipAttachment(cid []byte) []byte {
	attachment := &DeleteFlipAttachment{
		Cid: cid,
	}
	payload, _ := attachment.ToBytes()
	return payload
}

func ParseDeleteFlipAttachment(tx *types.Transaction) *DeleteFlipAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(DeleteFlipAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type CallContractAttachment struct {
	Method string
	Args   [][]byte
}

func (c *CallContractAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoCallContractAttachment{
		Method: c.Method,
		Args:   c.Args,
	}
	return proto.Marshal(protoAttachment)
}

func (c *CallContractAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoCallContractAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	c.Method = protoAttachment.Method
	c.Args = protoAttachment.Args
	return nil
}

func CreateCallContractAttachment(method string, args ...[]byte) *CallContractAttachment {
	attach := &CallContractAttachment{
		Method: method,
		Args:   args,
	}
	return attach
}

func ParseCallContractAttachment(tx *types.Transaction) *CallContractAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(CallContractAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type DeployContractAttachment struct {
	CodeHash common.Hash
	Args     [][]byte
}

func CreateDeployContractAttachment(codeHash common.Hash, args ...[]byte) *DeployContractAttachment {
	attach := &DeployContractAttachment{
		CodeHash: codeHash,
		Args:     args,
	}
	return attach
}

func (d *DeployContractAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoDeployContractAttachment{
		CodeHash: d.CodeHash.Bytes(),
		Args:     d.Args,
	}
	return proto.Marshal(protoAttachment)
}

func (d *DeployContractAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoDeployContractAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	d.CodeHash.SetBytes(protoAttachment.CodeHash)
	d.Args = protoAttachment.Args
	return nil
}

func ParseDeployContractAttachment(tx *types.Transaction) *DeployContractAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(DeployContractAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}

type TerminateContractAttachment struct {
	Args [][]byte
}

func CreateTerminateContractAttachment(args ...[]byte) *TerminateContractAttachment {
	attach := &TerminateContractAttachment{
		Args: args,
	}
	return attach
}

func (t *TerminateContractAttachment) ToBytes() ([]byte, error) {
	protoAttachment := &models.ProtoTerminateContractAttachment{
		Args: t.Args,
	}
	return proto.Marshal(protoAttachment)
}

func (t *TerminateContractAttachment) FromBytes(data []byte) error {
	protoAttachment := new(models.ProtoTerminateContractAttachment)
	if err := proto.Unmarshal(data, protoAttachment); err != nil {
		return err
	}
	t.Args = protoAttachment.Args
	return nil
}

func ParseTerminateContractAttachment(tx *types.Transaction) *TerminateContractAttachment {
	if len(tx.Payload) == 0 {
		return nil
	}
	attachment := new(TerminateContractAttachment)
	if err := attachment.FromBytes(tx.Payload); err != nil {
		return nil
	}
	return attachment
}
