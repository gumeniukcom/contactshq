package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Spec 005's own Independent Test tells an operator to place a .vcf in the backup directory by
// hand. Retention used a bare suffix check, so it would then delete that file as if it were its
// own output. Listing stays permissive — a hand-placed archive must remain restorable — but
// deletion is held to "did we write this?".
func TestIsMintedBackup_OnlyMatchesWhatThisServiceWrites(t *testing.T) {
	minted := []string{
		"backup-20260812-143000-123.vcf",
		"backup-20260812-143000-123.vcf.gz",
	}
	foreign := []string{
		"contacts-from-my-phone.vcf",
		"backup.vcf",
		"backup-2026-08-12.vcf",
		"backup-20260812-143000.vcf",
		"backup-20260812-143000-123.txt",
		"prefix-backup-20260812-143000-123.vcf",
	}

	for _, name := range minted {
		require.True(t, isMintedBackup(name), "%q is our own output", name)
		require.True(t, isBackupFilename(name), "%q must also be listable", name)
	}
	for _, name := range foreign {
		require.False(t, isMintedBackup(name), "%q was not written by this service", name)
	}

	// The two checks differ on purpose: a hand-placed archive is listed and restorable, and
	// never deleted.
	require.True(t, isBackupFilename("contacts-from-my-phone.vcf"))
	require.False(t, isMintedBackup("contacts-from-my-phone.vcf"))
}
