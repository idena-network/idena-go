package ipfs

import (
	"fmt"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/events"
	mg11 "github.com/ipfs/fs-repo-migrations/fs-repo-11-to-12/migration"
	gomigrate "github.com/ipfs/fs-repo-migrations/tools/go-migrate"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	mfsr "github.com/ipfs/go-ipfs/repo/fsrepo/migrations"
	"os"
)

type migrationState struct {
	eventBus eventbus.Bus
}

func (state *migrationState) Submit(message string) {
	state.eventBus.Publish(&events.IpfsMigrationProgressEvent{
		Message: message,
	})
}

func GetVersion(ipfsdir string) (int, error) {

	ver, err := mfsr.RepoVersion(ipfsdir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fsrepo.ErrNoVersion
		}
		return 0, err
	}
	return ver, nil
}

func runMigration(ipfsdir string, from int, to int, migrations map[int]gomigrate.Migration) error {
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

func doMigrate(ipfsdir string, from, to int, migrations map[int]gomigrate.Migration) error {
	if from > to {
		return fmt.Errorf("version cannot be reverted")
	}

	for cur := from; cur != to; cur += 1 {
		err := runMigration(ipfsdir, cur, cur+1, migrations)
		if err != nil {
			return err
		}
	}
	return nil
}

func Migrate(ipfsdir string, to int, eventBus eventbus.Bus) error {
	eventBus.Publish(&events.IpfsMigrationProgressEvent{
		Message: "preparing... this may take several minutes",
	})
	defer eventBus.Publish(&events.IpfsMigrationCompletedEvent{})
	migrations := map[int]gomigrate.Migration{
		11: &mg11.Migration{
			State: &migrationState{
				eventBus: eventBus,
			},
		},
	}
	version, err := GetVersion(ipfsdir)
	if err != nil {
		return err
	}
	return doMigrate(ipfsdir, version, to, migrations)
}
