package env

type GasCounter struct {
	UsedGas  int
	gasLimit int
}

func (g *GasCounter) AddGas(gas int) {
	g.UsedGas += gas
	if g.gasLimit >= 0 && g.gasLimit < g.UsedGas {
		panic("not enough gas")
	}
}

func (g *GasCounter) Reset(gasLimit int) {
	g.UsedGas = 0
	g.gasLimit = gasLimit
}
