package attachments

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/rlp"
)

type ShortAnswerAttachment struct {
	Answers []byte
	Proof   []byte
	Key     []byte
	Salt    []byte
}

func CreateShortAnswerAttachment(answers []byte, proof []byte, salt []byte, key *ecies.PrivateKey) []byte {
	attachment := &ShortAnswerAttachment{
		Answers: answers,
		Proof:   proof,
		Key:     crypto.FromECDSA(key.ExportECDSA()),
		Salt:    salt,
	}

	payload, _ := rlp.EncodeToBytes(attachment)

	return payload
}

func ParseShortAnswerAttachment(tx *types.Transaction) *ShortAnswerAttachment {
	return ParseShortAnswerBytesAttachment(tx.Payload)
}

func ParseShortAnswerBytesAttachment(payload []byte) *ShortAnswerAttachment {
	var ipfsAnswer ShortAnswerAttachment
	if err := rlp.Decode(bytes.NewReader(payload), &ipfsAnswer); err != nil {
		return nil
	}
	return &ipfsAnswer
}

type FlipSubmitAttachment struct {
	Cid  []byte
	Pair uint8
}

func CreateFlipSubmitAttachment(cid []byte, pair uint8) []byte {
	attachment := &FlipSubmitAttachment{
		Cid:  cid,
		Pair: pair,
	}
	payload, _ := rlp.EncodeToBytes(attachment)
	return payload
}

func ParseFlipSubmitAttachment(tx *types.Transaction) *FlipSubmitAttachment {
	var attachment FlipSubmitAttachment
	if err := rlp.Decode(bytes.NewReader(tx.Payload), &attachment); err != nil {
		return nil
	}
	return &attachment
}

type OnlineStatusAttachment struct {
	Online bool
}

func CreateOnlineStatusAttachment(online bool) []byte {
	attachment := &OnlineStatusAttachment{
		Online: online,
	}
	payload, _ := rlp.EncodeToBytes(attachment)
	return payload
}

func ParseOnlineStatusAttachment(tx *types.Transaction) *OnlineStatusAttachment {
	var attachment OnlineStatusAttachment
	if err := rlp.Decode(bytes.NewReader(tx.Payload), &attachment); err != nil {
		return nil
	}
	return &attachment
}

type BurnAttachment struct {
	Key string
}

func CreateBurnAttachment(key string) []byte {
	attachment := &BurnAttachment{
		Key: key,
	}
	payload, _ := rlp.EncodeToBytes(attachment)
	return payload
}

func ParseBurnAttachment(tx *types.Transaction) *BurnAttachment {
	var attachment BurnAttachment
	if err := rlp.Decode(bytes.NewReader(tx.Payload), &attachment); err != nil {
		return nil
	}
	return &attachment
}

type ChangeProfileAttachment struct {
	Hash []byte
}

func CreateChangeProfileAttachment(hash []byte) []byte {
	attachment := &ChangeProfileAttachment{
		Hash: hash,
	}
	payload, _ := rlp.EncodeToBytes(attachment)
	return payload
}

func ParseChangeProfileAttachment(tx *types.Transaction) *ChangeProfileAttachment {
	var attachment ChangeProfileAttachment
	if err := rlp.Decode(bytes.NewReader(tx.Payload), &attachment); err != nil {
		return nil
	}
	return &attachment
}

type DeleteFlipAttachment struct {
	Cid []byte
}

func CreateDeleteFlipAttachment(cid []byte) []byte {
	attachment := &DeleteFlipAttachment{
		Cid: cid,
	}
	payload, _ := rlp.EncodeToBytes(attachment)
	return payload
}

func ParseDeleteFlipAttachment(tx *types.Transaction) *DeleteFlipAttachment {
	var attachment DeleteFlipAttachment
	if err := rlp.Decode(bytes.NewReader(tx.Payload), &attachment); err != nil {
		return nil
	}
	return &attachment
}
