package ipfs

import (
	"fmt"
	gomigrate "github.com/ipfs/fs-repo-migrations/go-migrate"
	mg7 "github.com/ipfs/fs-repo-migrations/ipfs-7-to-8/migration"
	mg8 "github.com/ipfs/fs-repo-migrations/ipfs-8-to-9/migration"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	mfsr "github.com/ipfs/go-ipfs/repo/fsrepo/migrations"
	"os"
)

var migrations = map[int]gomigrate.Migration{
	7: &mg7.Migration{},
	8: &mg8.Migration{},
}

func GetVersion(ipfsdir string) (int, error) {

	ver, err := mfsr.RepoPath(ipfsdir).Version()
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fsrepo.ErrNoVersion
		}
		return 0, err
	}
	return ver, nil
}

func runMigration(ipfsdir string, from int, to int) error {
	fmt.Printf("===> Running migration %d to %d...\n", from, to)
	var err error
	opts := gomigrate.Options{}
	opts.Path = ipfsdir
	opts.Verbose = true

	if _, ok := migrations[from]; !ok {
		return fmt.Errorf("no migration found for version %v", to)
	}

	if to > from {
		err = migrations[from].Apply(opts)
	} else if to < from {
		err = migrations[to].Revert(opts)
	} else {
		// catch this earlier. expected invariant violated.
		err = fmt.Errorf("attempt to run migration to same version")
	}
	if err != nil {
		return fmt.Errorf("migration %d to %d failed: %s", from, to, err)
	}
	fmt.Printf("===> Migration %d to %d succeeded!\n", from, to)
	return nil
}

func doMigrate(ipfsdir string, from, to int) error {
	if from > to {
		return fmt.Errorf("version cannot be reverted")
	}

	for cur := from; cur != to; cur += 1 {
		err := runMigration(ipfsdir, cur, cur+1)
		if err != nil {
			return err
		}
	}
	return nil
}

func Migrate(ipfsdir string, to int) error {
	version, err := GetVersion(ipfsdir)
	if err != nil {
		return err
	}
	return doMigrate(ipfsdir, version, to)
}
