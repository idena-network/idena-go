package database

import (
	"github.com/tendermint/tendermint/libs/db"
	"gx/ipfs/QmPVkJMTeRC6iBByPWdrRkD3BE5UXsj5HPzb4kPqL186mS/testify/require"
	"testing"
)

func createDb(name string) *BackedMemDb {

	db, _ := db.NewGoLevelDB(name, "datadir")
	return NewBackedMemDb(db)
}

func TestBackedMemDb_Get(t *testing.T) {
	require := require.New(t)
	db := createDb("get.db")
	db.permanent.Set([]byte{0x1}, []byte{0x2})

	require.Equal(db.Get([]byte{0x1}), []byte{0x2})

	db.Set([]byte{0x1}, []byte{0x3})

	require.Equal(db.Get([]byte{0x1}), []byte{0x3})
	require.True(db.touched.Contains(string([]byte{0x1})))

	require.Equal(db.permanent.Get([]byte{0x1}), []byte{0x2})

	it := db.Iterator(nil, nil)
	require.True(it.Valid())
}

func TestBackedMemDb_Delete(t *testing.T) {
	require := require.New(t)
	db := createDb("delete.db")
	db.permanent.Set([]byte{0x1}, []byte{0x2})

	db.Delete([]byte{0x1})
	require.Equal([]byte(nil), db.Get([]byte{0x1}))
	require.True(db.touched.Contains(string([]byte{0x1})))

	require.Equal(db.permanent.Get([]byte{0x1}), []byte{0x2})

	it := db.Iterator(nil, nil)
	require.False(it.Valid())
}

func TestBackedMemDb_Iterator(t *testing.T) {
	require := require.New(t)
	db := createDb("iterator.db")

	db.permanent.Set([]byte{0x1}, []byte{0x5})
	db.permanent.Set([]byte{0x2}, []byte{0x3})
	db.permanent.Set([]byte{0x3}, []byte{0x4})
	db.permanent.Set([]byte{0x4}, []byte{0x4})

	db.Set([]byte{0x1}, []byte{0x1})
	db.Set([]byte{0x2}, []byte{0x2})
	db.Set([]byte{0x5}, []byte{0x6})
	db.Set([]byte{0x7}, []byte{0x8})
	db.Delete([]byte{0x4})

	assertions := []keyValue{
		{key: []byte{0x1}, value: []byte{0x1}},
		{key: []byte{0x2}, value: []byte{0x2}},
		{key: []byte{0x3}, value: []byte{0x4}},
		{key: []byte{0x5}, value: []byte{0x6}},
	}
	assertIterators(require, db, assertions, nil, []byte{0x6})
}

func TestBackedMemDb_Iterator2(t *testing.T) {
	require := require.New(t)
	db := createDb("iterator2.db")

	db.permanent.Set([]byte{0x1}, []byte{0x1})
	db.permanent.Set([]byte{0x2}, []byte{0x2})
	db.permanent.Set([]byte{0x3}, []byte{0x3})
	db.permanent.Set([]byte{0x4}, []byte{0x4})
	db.permanent.Set([]byte{0x5}, []byte{0x5})
	db.permanent.Set([]byte{0x6}, []byte{0x6})
	db.permanent.Set([]byte{0x7}, []byte{0x7})

	db.Set([]byte{0x1}, []byte{0x2})
	db.Set([]byte{0x2}, []byte{0x3})
	db.Set([]byte{0x6}, []byte{0x7})
	db.Delete([]byte{0x4})

	assertions := []keyValue{
		{key: []byte{0x1}, value: []byte{0x2}},
		{key: []byte{0x2}, value: []byte{0x3}},
		{key: []byte{0x3}, value: []byte{0x3}},
		{key: []byte{0x5}, value: []byte{0x5}},
		{key: []byte{0x6}, value: []byte{0x7}},
		{key: []byte{0x7}, value: []byte{0x7}},
	}
	assertIterators(require, db, assertions, nil, nil)
}

func TestBackedMemDb_NewBatch(t *testing.T) {
	require := require.New(t)
	db := createDb("newBatch.db")
	db.permanent.Set([]byte{0x1}, []byte{0x1})
	db.permanent.Set([]byte{0x2}, []byte{0x2})
	db.permanent.Set([]byte{0x3}, []byte{0x3})


	batch := db.NewBatch()
	batch.Set([]byte{0x1}, []byte{0x2})
	batch.Set([]byte{0x2}, []byte{0x3})
	batch.Delete([]byte{0x3})
	batch.Write()

	require.Equal(3, db.touched.Cardinality())
	require.Equal([]byte{0x2}, db.Get([]byte{0x1}))
	require.Equal([]byte{0x3}, db.Get([]byte{0x2}))
}

func assertIterators(require *require.Assertions, db *BackedMemDb, assertions []keyValue, start, end []byte) {
	cnt := 0
	it := db.Iterator(start, end)

	for i := 0; it.Valid(); it.Next() {
		require.Equal(assertions[i].key, it.Key())
		require.Equal(assertions[i].value, it.Value())
		i++
		cnt++
	}
	require.Equal(len(assertions), cnt)

	cnt = 0
	it = db.ReverseIterator(start, end)

	for i := len(assertions) - 1; it.Valid(); it.Next() {
		require.Equal(assertions[i].key, it.Key())
		require.Equal(assertions[i].value, it.Value())
		i--
		cnt++
	}
	require.Equal(len(assertions), cnt)
}

type keyValue struct {
	key   []byte
	value []byte
}
