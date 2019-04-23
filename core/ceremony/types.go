package ceremony

import "idena-go/common"

type dbAnswer struct {
	Addr common.Address
	Ans  []byte
}

type participant struct {
	PubKey    []byte
	Candidate bool
}
