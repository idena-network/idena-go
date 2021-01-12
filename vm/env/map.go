package env

import "github.com/idena-network/idena-go/common"

type Map struct {
	env    Env
	prefix []byte
	ctx    CallContext
}

// prefix length should be less common.MaxContractStoreKeyLength or prefix will be truncated
func NewMap(prefix []byte, env Env, ctx CallContext) *Map {
	if len(prefix) >= common.MaxContractStoreKeyLength {
		prefix = prefix[:common.MaxContractStoreKeyLength-2]
	}
	return &Map{prefix: prefix, env: env, ctx: ctx}
}

func (m *Map) formatKey(key []byte) []byte {
	return append(m.prefix, key...)
}

func (m *Map) Set(key []byte, value []byte) {
	m.env.SetValue(m.ctx, m.formatKey(key), value)
}

func (m *Map) Get(key []byte) []byte {
	return m.env.GetValue(m.ctx, m.formatKey(key))
}

func (m *Map) Remove(key []byte) {
	m.env.RemoveValue(m.ctx, m.formatKey(key))
}

func (m *Map) Iterate(f func(key []byte, value []byte) bool) {
	minKey := append(m.prefix[:0:0], m.prefix...)
	maxKey := append(m.prefix[:0:0], m.prefix...)
	for i := len(m.prefix); i < common.MaxContractStoreKeyLength; i++ {
		maxKey = append(maxKey, 0xFF)
	}

	m.env.Iterate(m.ctx, minKey, maxKey, func(key []byte, value []byte) bool {
		return f(key[len(m.prefix):], value)
	})
}
