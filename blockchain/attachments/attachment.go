package attachments

import (
	"bytes"
	"idena-go/blockchain/types"
	"idena-go/rlp"
)

type ShortAnswerAttachment struct {
	Answers []byte
	Pairs   []uint8
	Proof   []byte
	Key     []byte
}

func ParseShortAnswerAttachment(tx *types.Transaction) *ShortAnswerAttachment {
	var ipfsAnswer ShortAnswerAttachment
	if err := rlp.Decode(bytes.NewReader(tx.Payload), &ipfsAnswer); err != nil {
		return nil
	}
	return &ipfsAnswer
}
