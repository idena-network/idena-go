package env

type Map struct {
	env    Env
	prefix []byte
	ctx    CallContext
}

func NewMap(prefix []byte, env Env, ctx CallContext) *Map {
	return &Map{prefix: prefix, env: env, ctx: ctx}
}

func (m *Map) Set(key []byte, value []byte) {
	m.env.SetValue(m.ctx, append(m.prefix, key...), value)
}

func (m *Map) Get(key []byte) []byte {
	return m.env.GetValue(m.ctx, append(m.prefix, key...))
}

func (m *Map) Remove(key []byte) {
	m.env.RemoveValue(m.ctx, append(m.prefix, key...))
}

func (m *Map) Iterate(f func(key []byte, value []byte) bool) {
	emptySlice := make([]byte, 32)
	minKey := append(m.prefix, emptySlice...)
	maxKey := m.prefix
	for i := 0; i < 32; i++ {
		maxKey = append(maxKey, 0xFF)
	}

	m.env.Iterate(m.ctx, minKey, maxKey, func(key []byte, value []byte) bool {
		return f(key[len(m.prefix):], value)
	})
}
