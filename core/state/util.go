package state

import (
	"bytes"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/mholt/archiver"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"io"
	"io/ioutil"
	"strconv"
)

const (
	SnapshotBlockSize = 10000
)

func validationTxBitMask(txType types.TxType) byte {
	switch txType {
	case types.SubmitAnswersHashTx:
		return 1 << 0
	case types.SubmitShortAnswersTx:
		return 1 << 1
	case types.EvidenceTx:
		return 1 << 2
	case types.SubmitLongAnswersTx:
		return 1 << 3
	default:
		return 0
	}
}

func WriteTreeTo(sourceDb dbm.DB, height uint64, to io.Writer) (rootHash common.Hash, err error) {
	db := database.NewBackedMemDb(sourceDb)
	tree := NewMutableTree(db)
	if _, err := tree.LoadVersionForOverwriting(int64(height)); err != nil {
		return common.Hash{}, err
	}

	tar := archiver.Tar{
		MkdirAll:               true,
		OverwriteExisting:      false,
		ImplicitTopLevelFolder: false,
	}

	if err := tar.Create(to); err != nil {
		return common.Hash{}, err
	}

	it, err := db.Iterator(nil, nil)
	if err != nil {
		return common.Hash{}, err
	}
	defer it.Close()

	sb := new(models.ProtoSnapshotBlock)

	writeBlock := func(sb *models.ProtoSnapshotBlock, name string) error {

		data, _ := proto.Marshal(sb)

		return tar.Write(archiver.File{
			FileInfo: archiver.FileInfo{
				CustomName: name,
				FileInfo: &fakeFileInfo{
					size: int64(len(data)),
				},
			},
			ReadCloser: &readCloser{r: bytes.NewReader(data)},
		})
	}

	i := 0
	for ; it.Valid(); it.Next() {
		sb.Data = append(sb.Data, &models.ProtoSnapshotBlock_KeyValue{
			Key:   it.Key(),
			Value: it.Value(),
		})
		if len(sb.Data) >= SnapshotBlockSize {
			if err := writeBlock(sb, strconv.Itoa(i)); err != nil {
				return common.Hash{}, err
			}
			i++
			sb = new(models.ProtoSnapshotBlock)
		}
	}
	if len(sb.Data) > 0 {
		if err := writeBlock(sb, strconv.Itoa(i)); err != nil {
			return common.Hash{}, err
		}
	}
	return tree.WorkingHash(), tar.Close()
}

func ReadTreeFrom(pdb dbm.DB, height uint64, root common.Hash, from io.Reader) error {
	tar := archiver.Tar{
		MkdirAll:               true,
		OverwriteExisting:      false,
		ImplicitTopLevelFolder: false,
	}

	if err := tar.Open(from, 0); err != nil {
		return err
	}

	for file, err := tar.Read(); err == nil; file, err = tar.Read() {
		if data, err := ioutil.ReadAll(file); err != nil {
			common.ClearDb(pdb)
			return err
		} else {
			sb := new(models.ProtoSnapshotBlock)
			if err := proto.Unmarshal(data, sb); err != nil {
				common.ClearDb(pdb)
				return err
			}
			for _, pair := range sb.Data {
				pdb.Set(pair.Key, pair.Value)
			}
		}
	}
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		common.ClearDb(pdb)
		return err
	}

	if tree.WorkingHash() != root {
		common.ClearDb(pdb)
		return errors.New("wrong tree root")
	}
	if !tree.ValidateTree() {
		common.ClearDb(pdb)
		return errors.New("corrupted tree")
	}
	return nil
}
