package decimal

import "github.com/cockroachdb/apd"

type Decimal struct {
	underlined *apd.Decimal
}

var apdContext = apd.Context{
	MaxExponent: apd.BaseContext.MaxExponent,
	MinExponent: apd.MinExponent,
	Precision:   16,
	Rounding:    apd.BaseContext.Rounding,
	Traps:       apd.BaseContext.Traps,
}

func New(m int64, e int32) *Decimal {
	return &Decimal{apd.New(m, e)}
}

func (d *Decimal) Div(d2 *Decimal) *Decimal {
	r := new(apd.Decimal)
	apdContext.Quo(r, d.underlined, d2.underlined)
	return &Decimal{r}
}

func (d *Decimal) DivWithPrecision(d2 *Decimal, precision uint32) *Decimal {
	apdContext := apd.Context{
		MaxExponent: apd.BaseContext.MaxExponent,
		MinExponent: apd.MinExponent,
		Precision:   precision,
		Rounding:    apd.BaseContext.Rounding,
		Traps:       apd.BaseContext.Traps,
	}
	r := new(apd.Decimal)
	apdContext.Quo(r, d.underlined, d2.underlined)
	return &Decimal{r}
}
