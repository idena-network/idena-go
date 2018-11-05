package api

import "idena-go/consensus"

type DnaApi struct {
	engine *consensus.Engine
}

func NewDnaApi(engine *consensus.Engine) *DnaApi {
	return &DnaApi{engine: engine}
}

type State struct {
	Name string `json:"name"`
}

func (api *DnaApi) State() State {
	return State{
		Name: api.engine.GetState(),
	}
}
