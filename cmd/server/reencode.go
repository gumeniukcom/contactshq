package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	gvcard "github.com/emersion/go-vcard"
	"github.com/uptrace/bun"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

// reencodeBatchSize bounds how many rows are held and written at a time.
const reencodeBatchSize = 500

// runReencodeVCards rewrites stored vCards with the current encoder.
//
// This is deliberately a command and not a migration. `applyMigration` runs each file inside
// a single transaction, and on SQLite the pool is one connection wide: a bulk UPDATE would
// hold that connection for the duration, the container health check (10s start period) would
// call the server unhealthy, and compose would restart it in the middle of the transaction.
// A migration also offers no dry run and no way to decline.
func runReencodeVCards(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reencode-vcards", flag.ContinueOnError)
	fs.SetOutput(stderr)
	apply := fs.Bool("apply", false, "actually write the changes (default is a dry run)")
	reconcile := fs.Bool("reconcile-sync-state", false,
		"required with --apply: bring sync_states in line with the rewritten cards")
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: contactshq reencode-vcards [--apply --reconcile-sync-state]

Rewrites every stored vCard with the current encoder, fixing values the old one escaped
incorrectly (photos, category separators, embedded newlines).

Runs as a dry run unless --apply is given. --apply requires --reconcile-sync-state.
`)
		fs.PrintDefaults()
	}

	if _, err := parseInterleaved(fs, args); err != nil {
		return exitUsage
	}

	// Rewriting cards without fixing sync_states is worse than doing nothing: the engine
	// would see the entire address book as locally modified and push all of it outward on the
	// next export or two-way run. Making the flag mandatory removes the chance to find that
	// out afterwards.
	if *apply && !*reconcile {
		fmt.Fprintln(stderr, `--apply requires --reconcile-sync-state.

Rewriting cards changes contacts.etag, while sync_states.local_etag still holds the old one.
The sync engine reads that as "every contact changed locally" and the next export or two_way
run rewrites the whole address book on Google or CardDAV.`)
		return exitUsage
	}

	db, _, code := openCLIDatabase(stderr)
	if code != exitOK {
		return code
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	if *apply {
		fmt.Fprint(stdout, `About to rewrite stored vCards.

There is no undo. Take a database dump first if you have not.
Every CardDAV client will re-download the affected cards: each gets a new ETag, and the
address book's change counter — the CTag your devices poll — advances with them, so a
device that only watches the CTag learns about the repair too. Stop your pipelines before
this run and start them after it.

`)
	}

	changed, scanned, err := reencodeContacts(ctx, db, *apply, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "failed to re-encode contacts: %v\n", err)
		return exitFailure
	}

	fmt.Fprintf(stdout, "contacts: %d of %d need rewriting\n", changed, scanned)

	if !*apply {
		fmt.Fprintln(stdout, "\nDry run — nothing was written. Re-run with --apply --reconcile-sync-state.")
		return exitOK
	}

	reconciled, err := reconcileSyncStates(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "cards were rewritten but sync state could not be reconciled: %v\n", err)
		fmt.Fprintln(stderr, "DO NOT run a pipeline until this succeeds — it would push the whole address book.")
		return exitFailure
	}
	fmt.Fprintf(stdout, "sync states reconciled: %d\n", reconciled)

	return exitOK
}

// reencodeVCard decodes a stored card and writes it back with the current encoder, returning
// the result and whether it differs from the input.
func reencodeVCard(stored string) (string, bool, error) {
	card, err := gvcard.NewDecoder(strings.NewReader(stored)).Decode()
	if err != nil {
		return "", false, err
	}

	var sb strings.Builder
	if err := vcardpkg.EncodeCard(&sb, card); err != nil {
		return "", false, err
	}
	out := sb.String()
	return out, out != stored, nil
}

// reencodeContacts walks every contact in batches. A contact whose card cannot be decoded is
// counted and left alone: a repair command must not be the thing that loses data.
func reencodeContacts(ctx context.Context, db *bun.DB, apply bool, stdout io.Writer) (changed, scanned int, err error) {
	var undecodable int

	for offset := 0; ; offset += reencodeBatchSize {
		var contacts []*domain.Contact
		err := db.NewSelect().
			Model(&contacts).
			Order("id ASC").
			Limit(reencodeBatchSize).
			Offset(offset).
			Scan(ctx)
		if err != nil {
			return changed, scanned, err
		}
		if len(contacts) == 0 {
			break
		}

		// Rewrites are grouped by address book so each book's change counter advances once per
		// batch and every card rewritten in it shares that sequence — which is what a bulk write
		// means. Bumping it is not optional: the counter IS the collection's CTag, so without it
		// a CTag-polling client (iOS is one) never asks again and never sees the repair this
		// command exists to perform.
		pending := map[string][]*domain.Contact{}

		for _, c := range contacts {
			scanned++

			rewritten, differs, decErr := reencodeVCard(c.VCardData)
			if decErr != nil {
				undecodable++
				continue
			}
			if !differs {
				continue
			}
			changed++

			if !apply {
				continue
			}

			c.VCardData = rewritten
			pending[c.AddressBookID] = append(pending[c.AddressBookID], c)
		}

		for abID, rewritten := range pending {
			if err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				seq, err := repository.BumpChangeSeq(ctx, tx, abID)
				if err != nil {
					return fmt.Errorf("bump change_seq for address book %s: %w", abID, err)
				}
				for _, c := range rewritten {
					if _, err := tx.NewUpdate().
						Model((*domain.Contact)(nil)).
						Set("vcard_data = ?", c.VCardData).
						Set("etag = ?", service.ContactETag(c.VCardData)).
						Set("change_seq = ?", seq).
						Where("id = ?", c.ID).
						Exec(ctx); err != nil {
						return fmt.Errorf("update contact %s: %w", c.ID, err)
					}
				}
				return nil
			}); err != nil {
				return changed, scanned, err
			}
		}

		fmt.Fprintf(stdout, "  scanned %d, needing rewrite %d\n", scanned, changed)
	}

	if undecodable > 0 {
		fmt.Fprintf(stdout, "  %d card(s) could not be decoded and were left untouched\n", undecodable)
	}
	return changed, scanned, nil
}

// reconcileSyncStates re-renders the stored merge anchor with the new encoder and points
// local_etag at what the contact now hashes to.
//
// remote_etag is deliberately untouched: it is the remote server's opaque value and nothing
// here changed the remote side.
func reconcileSyncStates(ctx context.Context, db *bun.DB) (int, error) {
	var states []*domain.SyncState
	if err := db.NewSelect().Model(&states).Scan(ctx); err != nil {
		return 0, err
	}

	updated := 0
	for _, st := range states {
		var contact domain.Contact
		hasContact := false
		if st.LocalID != "" {
			err := db.NewSelect().Model(&contact).Where("id = ?", st.LocalID).Scan(ctx)
			hasContact = err == nil
		}

		newBase := st.BaseVCard
		if st.BaseVCard != "" {
			if rewritten, _, err := reencodeVCard(st.BaseVCard); err == nil {
				newBase = rewritten
			}
		}

		q := db.NewUpdate().Model((*domain.SyncState)(nil)).Where("id = ?", st.ID)
		q = q.Set("base_vcard = ?", newBase).
			Set("content_hash = ?", chqsync.ContentHash(newBase))
		if hasContact {
			q = q.Set("local_etag = ?", contact.ETag)
		}

		if _, err := q.Exec(ctx); err != nil {
			return updated, fmt.Errorf("update sync state %s: %w", st.ID, err)
		}
		updated++
	}

	return updated, nil
}
