package keywords

import (
	"encoding/json"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
)

type Keyword struct {
	Name string
	Desc string
}

var list []Keyword

func init() {
	list = make([]Keyword, 0, 3990)
	data, _ := Asset("keywords.json")
	if err := json.Unmarshal(data, &list); err != nil {
		log.Warn("cannot parse keywords.json", "err", err)
	}
}

func Get(index int) (Keyword, error) {
	if index >= len(list) || index < 0 {
		return Keyword{}, errors.New("index is out of range")
	}
	return list[index], nil
}
