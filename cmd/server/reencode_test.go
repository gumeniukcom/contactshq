package main

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

const legacyCard = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:legacy-1\r\nFN:Legacy Card\r\n" +
	"CATEGORIES:work\\,friends\r\nPHOTO:data:image/jpeg;base64\\,/9j/4AAQ\r\nEND:VCARD\r\n"

func newReencodeDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.Migrate(context.Background(), db))
	return db
}

func TestReencodeVCard_FixesLegacyEscapingAndIsIdempotent(t *testing.T) {
	fixed, changed, err := reencodeVCard(legacyCard)
	require.NoError(t, err)
	require.True(t, changed)

	require.Contains(t, fixed, "CATEGORIES:work,friends")
	require.Contains(t, fixed, "PHOTO:data:image/jpeg;base64,/9j/4AAQ")
	require.NotContains(t, fixed, `\,`)

	again, changedAgain, err := reencodeVCard(fixed)
	require.NoError(t, err)
	require.False(t, changedAgain, "a second pass must find nothing to do")
	require.Equal(t, fixed, again)
}

// A card the decoder cannot read must be left exactly as it is: a repair command is not
// allowed to be the thing that loses data.
func TestReencodeContacts_LeavesUndecodableCardsAlone(t *testing.T) {
	db := newReencodeDB(t)
	ctx := context.Background()

	require.NoError(t, seedAddressBook(ctx, db))
	broken := "this is not a vCard at all"
	require.NoError(t, insertContact(ctx, db, "c-broken", "broken", broken))

	var out bytes.Buffer
	changed, scanned, err := reencodeContacts(ctx, db, true, &out)
	require.NoError(t, err)
	require.Equal(t, 1, scanned)
	require.Zero(t, changed)
	require.Contains(t, out.String(), "could not be decoded")

	var stored domain.Contact
	require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", "c-broken").Scan(ctx))
	require.Equal(t, broken, stored.VCardData, "an unreadable card must not be touched")
}

func TestReencodeContacts_DryRunWritesNothing(t *testing.T) {
	db := newReencodeDB(t)
	ctx := context.Background()

	require.NoError(t, seedAddressBook(ctx, db))
	require.NoError(t, insertContact(ctx, db, "c1", "legacy-1", legacyCard))

	var out bytes.Buffer
	changed, scanned, err := reencodeContacts(ctx, db, false, &out)
	require.NoError(t, err)
	require.Equal(t, 1, scanned)
	require.Equal(t, 1, changed)

	var stored domain.Contact
	require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", "c1").Scan(ctx))
	require.Equal(t, legacyCard, stored.VCardData, "a dry run must not write")
}

func TestReencodeContacts_ApplyRewritesCardAndETag(t *testing.T) {
	db := newReencodeDB(t)
	ctx := context.Background()

	require.NoError(t, seedAddressBook(ctx, db))
	require.NoError(t, insertContact(ctx, db, "c1", "legacy-1", legacyCard))

	var out bytes.Buffer
	_, _, err := reencodeContacts(ctx, db, true, &out)
	require.NoError(t, err)

	var stored domain.Contact
	require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", "c1").Scan(ctx))
	require.NotContains(t, stored.VCardData, `\,`)
	require.Equal(t, service.ContactETag(stored.VCardData), stored.ETag,
		"the ETag must match what the write paths would have stored, or clients never match it")
}

// Without this, contacts.etag moves while sync_states.local_etag does not, and the engine
// reads the whole address book as locally modified — the next export pushes all of it out.
func TestReconcileSyncStates_PointsLocalETagAtTheRewrittenCard(t *testing.T) {
	db := newReencodeDB(t)
	ctx := context.Background()

	require.NoError(t, seedAddressBook(ctx, db))
	require.NoError(t, insertContact(ctx, db, "c1", "legacy-1", legacyCard))

	staleState := &domain.SyncState{
		ID:           "s1",
		UserID:       "u1",
		ProviderType: "carddav",
		LocalID:      "c1",
		RemoteID:     "remote-1",
		RemoteETag:   `"remote-etag"`,
		LocalETag:    service.ContactETag(legacyCard),
		ContentHash:  chqsync.ContentHash(legacyCard),
		BaseVCard:    legacyCard,
	}
	_, err := db.NewInsert().Model(staleState).Exec(ctx)
	require.NoError(t, err)

	var out bytes.Buffer
	_, _, err = reencodeContacts(ctx, db, true, &out)
	require.NoError(t, err)

	reconciled, err := reconcileSyncStates(ctx, db)
	require.NoError(t, err)
	require.Equal(t, 1, reconciled)

	var contact domain.Contact
	require.NoError(t, db.NewSelect().Model(&contact).Where("id = ?", "c1").Scan(ctx))

	var state domain.SyncState
	require.NoError(t, db.NewSelect().Model(&state).Where("id = ?", "s1").Scan(ctx))

	require.Equal(t, contact.ETag, state.LocalETag,
		"local_etag must match the rewritten contact, or the engine sees a local change")
	require.NotContains(t, state.BaseVCard, `\,`, "the merge anchor must be re-encoded too")
	require.Equal(t, chqsync.ContentHash(state.BaseVCard), state.ContentHash,
		"content_hash must agree with the anchor it describes")
	require.Equal(t, `"remote-etag"`, state.RemoteETag,
		"remote_etag describes the remote side and nothing here touched it")
}

// --apply without --reconcile-sync-state is refused: doing the first half alone is worse than
// doing nothing, because the next pipeline run rewrites the remote address book.
func TestRunReencodeVCards_ApplyRequiresReconcile(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := runReencodeVCards([]string{"--apply"}, strings.NewReader(""), &out, &errBuf)

	require.Equal(t, exitUsage, code)
	require.Contains(t, errBuf.String(), "--reconcile-sync-state")
	require.Contains(t, errBuf.String(), "rewrites the whole address book")
}

func seedAddressBook(ctx context.Context, db *bun.DB) error {
	if _, err := db.NewInsert().Model(&domain.User{
		ID: "u1", Email: "owner@example.com", PasswordHash: "x", Role: "admin",
	}).Exec(ctx); err != nil {
		return err
	}
	_, err := db.NewInsert().Model(&domain.AddressBook{
		ID: "ab1", UserID: "u1", Name: "Contacts",
	}).Exec(ctx)
	return err
}

func insertContact(ctx context.Context, db *bun.DB, id, uid, card string) error {
	_, err := db.NewInsert().Model(&domain.Contact{
		ID:            id,
		AddressBookID: "ab1",
		UID:           uid,
		VCardData:     card,
		ETag:          service.ContactETag(card),
	}).Exec(ctx)
	return err
}

// The command exists to repair cards a CardDAV client is already showing wrongly — corrupted
// iOS photos are the worked example. Rewriting them without advancing the address book's
// change counter meant a CTag-polling device never asked again, so the repair reached the
// database and no client at all, while the command printed the opposite to the operator.
func TestReencodeContacts_ApplyAdvancesTheChangeCounter(t *testing.T) {
	db := newReencodeDB(t)
	ctx := context.Background()

	require.NoError(t, seedAddressBook(ctx, db))
	require.NoError(t, insertContact(ctx, db, "c1", "legacy-1", legacyCard))

	var before domain.AddressBook
	require.NoError(t, db.NewSelect().Model(&before).Where("id = ?", "ab1").Scan(ctx))

	var out bytes.Buffer
	changed, _, err := reencodeContacts(ctx, db, true, &out)
	require.NoError(t, err)
	require.Equal(t, 1, changed)

	var after domain.AddressBook
	require.NoError(t, db.NewSelect().Model(&after).Where("id = ?", "ab1").Scan(ctx))
	require.Greater(t, after.ChangeSeq, before.ChangeSeq,
		"the CTag must move, or a polling client never learns of the repair")

	var stored domain.Contact
	require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", "c1").Scan(ctx))
	require.Equal(t, after.ChangeSeq, stored.ChangeSeq,
		"the rewritten contact must carry that sequence, or sync-collection skips it")
}

// A dry run must not move the counter either — it writes nothing at all.
func TestReencodeContacts_DryRunLeavesTheChangeCounterAlone(t *testing.T) {
	db := newReencodeDB(t)
	ctx := context.Background()

	require.NoError(t, seedAddressBook(ctx, db))
	require.NoError(t, insertContact(ctx, db, "c1", "legacy-1", legacyCard))

	var before domain.AddressBook
	require.NoError(t, db.NewSelect().Model(&before).Where("id = ?", "ab1").Scan(ctx))

	var out bytes.Buffer
	_, _, err := reencodeContacts(ctx, db, false, &out)
	require.NoError(t, err)

	var after domain.AddressBook
	require.NoError(t, db.NewSelect().Model(&after).Where("id = ?", "ab1").Scan(ctx))
	require.Equal(t, before.ChangeSeq, after.ChangeSeq)
}
