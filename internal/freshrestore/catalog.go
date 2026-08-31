package freshrestore

import (
	"context"

	"github.com/bprendie/weazlback/internal/catalog"
)

func RebuildRecoveryCatalog(ctx context.Context, kit string, passphrase []byte, destination, machineID string) (string, error) {
	session, err := OpenSessionDestinationAt(kit, passphrase, destination, "")
	if err != nil {
		return "", err
	}
	defer session.Close()
	if err := catalog.Refresh(ctx, session.Vault, session.Destination.ID, session.Repository, machineID, "core", "home", "heavy"); err != nil {
		return "", err
	}
	return catalog.Path(session.Destination.ID)
}
