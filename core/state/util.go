package state

import (
	"bytes"
	"github.com/cosmos/iavl"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/mholt/archiver/v3"
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

func WriteTreeTo2(sourceDb dbm.DB, height uint64, to io.Writer) (common.Hash, error) {
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

	exporter := tree.GetImmutable().Exporter()
	defer exporter.Close()
	nodes := new(models.ProtoSnapshotNodes)
	i := 0

	writeBlock := func(sb *models.ProtoSnapshotNodes, name string) error {

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

	for {
		node, err := exporter.Next()
		if err != nil {
			break
		}
		nodes.Nodes = append(nodes.Nodes, &models.ProtoSnapshotNodes_Node{
			Key:        node.Key,
			Height:     uint32(node.Height),
			Value:      node.Value,
			Version:    uint64(node.Version),
			EmptyValue: node.Value != nil && len(node.Value) == 0,
		})
		if len(nodes.Nodes) >= SnapshotBlockSize {
			if err := writeBlock(nodes, strconv.Itoa(i)); err != nil {
				return common.Hash{}, err
			}
			i++
			nodes = new(models.ProtoSnapshotNodes)
		}
	}
	if len(nodes.Nodes) > 0 {
		if err := writeBlock(nodes, strconv.Itoa(i)); err != nil {
			return common.Hash{}, err
		}
	}
	return tree.WorkingHash(), tar.Close()
}

func ReadTreeFrom2(pdb *dbm.PrefixDB, height uint64, root common.Hash, from io.Reader) error {
	tar := archiver.Tar{
		MkdirAll:               true,
		OverwriteExisting:      false,
		ImplicitTopLevelFolder: false,
	}

	if err := tar.Open(from, 0); err != nil {
		return err
	}

	tree := NewMutableTree(pdb)
	importer, err := tree.Importer(int64(height))
	if err != nil {
		return err
	}
	defer importer.Close()

	for file, err := tar.Read(); err == nil; file, err = tar.Read() {
		if data, err := ioutil.ReadAll(file); err != nil {
			common.ClearDb(pdb)
			return err
		} else {
			sb := new(models.ProtoSnapshotNodes)
			if err := proto.Unmarshal(data, sb); err != nil {
				common.ClearDb(pdb)
				return err
			}
			for _, node := range sb.Nodes {

				exportNode := &iavl.ExportNode{
					Key:     node.Key,
					Value:   node.Value,
					Version: int64(node.Version),
					Height:  int8(node.Height),
				}

				if node.EmptyValue {
					exportNode.Value = make([]byte, 0)
				}

				importer.Add(exportNode)
			}
		}
	}
	if err := importer.Commit(); err != nil {
		common.ClearDb(pdb)
		return err
	}

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
