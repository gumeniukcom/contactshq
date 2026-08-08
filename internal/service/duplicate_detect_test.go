package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

func seedContact(repo *mockContactRepo, id, first, last, email, phone string) {
	c := &domain.Contact{
		ID: id, AddressBookID: testAddressBookID, UID: id,
		FirstName: first, LastName: last, Email: email, Phone: phone,
	}
	repo.contacts[id] = c
	repo.byUID[testAddressBookID+":"+id] = c
}

// seedChildEmail attaches a contact_emails row. The contact's own `email` column is its
// primary address; everything else a person has lives only here.
func seedChildEmail(repo *mockContactRepo, contactID, value string) {
	repo.dedupEmails = append(repo.dedupEmails, domain.ContactValueRef{ContactID: contactID, Value: value})
}

// seedChildPhone attaches a contact_phones row.
func seedChildPhone(repo *mockContactRepo, contactID, value string) {
	repo.dedupPhones = append(repo.dedupPhones, domain.ContactValueRef{ContactID: contactID, Value: value})
}

func newDetector(t *testing.T) (*service.DuplicateDetector, *mockContactRepo, *mockDupRepo) {
	t.Helper()
	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	dupRepo := newMockDupRepo()
	return service.NewDuplicateDetector(contactRepo, abRepo, dupRepo, zap.NewNop()), contactRepo, dupRepo
}

// pairKey renders a stored duplicate as a comparable fixture line.
//
// match_reasons now carries the matched value alongside the code, so the fixture pins both:
// the codes are what the old all-pairs scorer produced, and the values are what task 4.5
// added so the UI can say "Same email: a@b.c" instead of inventing it from the contacts.
func pairKey(t *testing.T, d *domain.PotentialDuplicate) string {
	t.Helper()
	var parsed []service.MatchReason
	require.NoError(t, json.Unmarshal([]byte(d.MatchReasons), &parsed))

	reasons := make([]string, 0, len(parsed))
	for _, r := range parsed {
		if r.Value != "" {
			reasons = append(reasons, r.Code+"("+r.Value+")")
			continue
		}
		reasons = append(reasons, r.Code)
	}
	sort.Strings(reasons)

	a, b := d.ContactAID, d.ContactBID
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%s+%s score=%.2f reasons=%v", a, b, d.Score, reasons)
}

func detectedPairs(t *testing.T, dupRepo *mockDupRepo) []string {
	t.Helper()
	out := make([]string, 0, len(dupRepo.records))
	for _, d := range dupRepo.records {
		out = append(out, pairKey(t, d))
	}
	sort.Strings(out)
	return out
}

// Characterisation of the CURRENT detector, to be compared against after task 4.5 replaces
// the O(N²) scan with key bucketing.
//
// Two properties are pinned deliberately, not just the set of pairs:
//
//   - The 0.8 threshold is unreachable by name alone: name_exact scores 0.7 and name_similar
//     0.5, so two identical names with no shared email or phone are NOT reported. Anyone
//     changing the bucketing has to decide consciously whether to keep that.
//   - A pair that qualifies on phone additionally carries name_exact / name_similar in its
//     reasons, because scoreContacts keeps appending after the phone match sets the score.
//     Bucketing by key would drop those extra reasons silently.
func TestDetect_CharacterisesCurrentBehaviour(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)

	// Exact email → score 1.0, single reason.
	seedContact(contactRepo, "c1", "Ada", "Lovelace", "ada@example.com", "+1 555 0001")
	seedContact(contactRepo, "c2", "A.", "Lovelace", "ADA@example.com", "+1 555 9999")

	// Same phone in different formats, same name → phone_match plus name_exact.
	seedContact(contactRepo, "c3", "Grace", "Hopper", "grace@example.com", "+1 (555) 0002")
	seedContact(contactRepo, "c4", "Grace", "Hopper", "g.hopper@example.com", "15550002")

	// Same phone, near-identical name → phone_match plus name_similar.
	seedContact(contactRepo, "c5", "Alan", "Turing", "alan@example.com", "+1 555 0003")
	seedContact(contactRepo, "c6", "Alan", "Turring", "a.turing@example.com", "+1 555 0003")

	// Identical names, nothing else in common → below the threshold, NOT reported.
	seedContact(contactRepo, "c7", "John", "Smith", "john1@example.com", "+1 555 0007")
	seedContact(contactRepo, "c8", "John", "Smith", "john2@example.com", "+1 555 0008")

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 8, res.Checked)

	want := []string{
		"c1+c2 score=1.00 reasons=[email_match(ada@example.com)]",
		"c3+c4 score=0.80 reasons=[name_exact(Grace Hopper) phone_match(15550002)]",
		"c5+c6 score=0.80 reasons=[name_similar(Alan Turing) phone_match(15550003)]",
	}
	require.Equal(t, want, detectedPairs(t, dupRepo))
	require.Equal(t, 3, res.Found)

	// Spelled out so a future change to the threshold has to confront it explicitly.
	for _, pair := range detectedPairs(t, dupRepo) {
		require.NotContains(t, pair, "c7", "identical names alone must not reach the 0.8 threshold")
		require.NotContains(t, pair, "c8")
	}
}

// A pair already recorded is not recorded twice, whichever way round it is stored.
func TestDetect_DoesNotDuplicateAnExistingPair(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)
	seedContact(contactRepo, "c1", "Ada", "Lovelace", "ada@example.com", "")
	seedContact(contactRepo, "c2", "Ada", "Lovelace", "ada@example.com", "")

	first, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 1, first.Found)
	require.Equal(t, 1, dupRepo.creates)

	second, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Zero(t, second.Found, "a known pair must not be recorded again")
	require.Equal(t, 1, dupRepo.creates)
}

func TestDetect_EmptyAddressBook(t *testing.T) {
	det, _, dupRepo := newDetector(t)

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Zero(t, res.Checked)
	require.Zero(t, res.Found)
	require.Zero(t, dupRepo.creates)
}

// Baseline for task 4.5. Run with:
//
//	go test ./internal/service/ -run XXX -bench Detect -benchmem -benchtime 1x
//
// Worst case: no pair shares a key, so nothing short-circuits.
//
// Before bucketing (all-pairs scan, a levenshtein matrix allocated per comparison):
//
//	BenchmarkDetect/1000_contacts-8       0.39 s/op    136 MB/op    3.5M allocs/op
//	BenchmarkDetect/10000_contacts-8     38.9  s/op   13.6 GB/op   350M allocs/op
//
// After (measured 2026-08-06):
//
//	BenchmarkDetect/1000_contacts-8      0.6 ms/op    549 KB/op     7.0k allocs/op
//	BenchmarkDetect/10000_contacts-8     8.0 ms/op    4.9 MB/op    70.2k allocs/op
//
// The shape is what matters, not the multiple: ten times the contacts used to cost a hundred
// times the work, and now costs about thirteen. A scheduled scan on ten thousand contacts went
// from occupying a worker for most of a minute to eight milliseconds.
//
// Use -benchtime 1x: the old figures make the default run unbearable, and one iteration is
// enough to see the shape.
//
// 2026-08-07, reading the child tables: this fixture seeds no child rows, so its bucketing
// work is unchanged, but the `seen` set that keeps a contact out of one bucket twice costs
// memory whether or not there is anything to de-duplicate. Same machine, same session,
// -benchtime 10x -count 3, so the two rows are genuinely comparable — the 2026-08-06 figures
// above are from another machine and are not:
//
//	10000_contacts before:  ~10 ms/op   4.9 MB/op   70.2k allocs/op
//	10000_contacts after:   ~10 ms/op   7.3 MB/op   90.3k allocs/op
//
// Time is unchanged inside the noise; memory is up by half. That is the price of the guard,
// and it is paid on every scan.
func BenchmarkDetect(b *testing.B) {
	for _, size := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("%d_contacts", size), func(b *testing.B) {
			contactRepo := newMockContactRepo()
			for i := 0; i < size; i++ {
				seedContact(contactRepo,
					fmt.Sprintf("c%05d", i),
					fmt.Sprintf("First%05d", i),
					fmt.Sprintf("Last%05d", i),
					fmt.Sprintf("user%05d@example.com", i),
					fmt.Sprintf("+1555%06d", i),
				)
			}
			abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
			det := service.NewDuplicateDetector(contactRepo, abRepo, newMockDupRepo(), zap.NewNop())

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := det.Detect(ctx, "u1"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDetectWithSecondaryValues is BenchmarkDetect's fixture plus two child rows per
// contact — a second email and a second phone, the shape of a contact imported from a phone.
//
// WHAT IT MEASURES, AND WHAT IT DOES NOT: like BenchmarkDetect it runs against
// mockContactRepo, an in-memory map. It issues no query and says nothing about the I/O cost of
// reading contact_emails and contact_phones; that argument is made in its own terms on
// ListDedupValues. What it bounds is the Go-side work that genuinely grows here — three times
// the bucket insertions and the `seen` set that keeps a repeated value out of a bucket.
//
// Run it alongside BenchmarkDetect, whose fixture is left untouched so the two are comparable:
//
//	go test ./internal/service/ -run XXX -bench Detect -benchmem -benchtime 10x -count 3
//
// Measured 2026-08-07, Apple M3, median of three:
//
//	BenchmarkDetectWithSecondaryValues/1000_contacts-8     1.8 ms/op   1.9 MB/op   23.1k allocs/op
//	BenchmarkDetectWithSecondaryValues/10000_contacts-8   25   ms/op  16.8 MB/op    231k allocs/op
//
// The shape is what the constraint was: ten times the contacts costs about fourteen times the
// work, the same near-linear growth the flat-column scan shows on this machine (~0.8 ms →
// ~10 ms), at roughly 2.4× its constant for three times the values. A figure that came back
// super-linear would mean the union step had reintroduced all-pairs comparison, and this
// change would have to stop.
//
// -benchtime 10x rather than the 1x above: these figures are milliseconds, one iteration of
// them is mostly scheduler noise, and the whole claim here is about a ratio.
func BenchmarkDetectWithSecondaryValues(b *testing.B) {
	for _, size := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("%d_contacts", size), func(b *testing.B) {
			contactRepo := newMockContactRepo()
			for i := 0; i < size; i++ {
				id := fmt.Sprintf("c%05d", i)
				seedContact(contactRepo, id,
					fmt.Sprintf("First%05d", i),
					fmt.Sprintf("Last%05d", i),
					fmt.Sprintf("user%05d@example.com", i),
					fmt.Sprintf("+1555%06d", i),
				)
				seedChildEmail(contactRepo, id, fmt.Sprintf("user%05d@example.com", i))
				seedChildEmail(contactRepo, id, fmt.Sprintf("second%05d@example.org", i))
				seedChildPhone(contactRepo, id, fmt.Sprintf("+1555%06d", i))
				seedChildPhone(contactRepo, id, fmt.Sprintf("+1777%06d", i))
			}
			abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
			det := service.NewDuplicateDetector(contactRepo, abRepo, newMockDupRepo(), zap.NewNop())

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := det.Detect(ctx, "u1"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Contacts sharing a key are compared; contacts sharing nothing are never looked at. Three
// records with one email produce all three pairs.
func TestDetect_BucketsByKey(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)

	seedContact(contactRepo, "c1", "Ada", "L", "shared@example.com", "")
	seedContact(contactRepo, "c2", "Ada", "Lovelace", "SHARED@example.com", "")
	seedContact(contactRepo, "c3", "A", "Lovelace", "shared@EXAMPLE.com", "")
	// No key in common with anyone.
	seedContact(contactRepo, "c4", "Grace", "Hopper", "grace@example.com", "+15559999")

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 3, res.Found, "three contacts sharing one email make three pairs")

	for _, pair := range detectedPairs(t, dupRepo) {
		require.NotContains(t, pair, "c4", "a contact sharing no key must not be paired")
	}
}

// Pairs are stored with the smaller id first, whichever order they were found in. The unique
// index in migration 024 depends on it.
func TestDetect_NormalisesPairOrder(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)
	seedContact(contactRepo, "zzz", "Ada", "L", "shared@example.com", "")
	seedContact(contactRepo, "aaa", "Ada", "L", "shared@example.com", "")

	_, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)

	require.Len(t, dupRepo.records, 1)
	for _, d := range dupRepo.records {
		require.Equal(t, "aaa", d.ContactAID)
		require.Equal(t, "zzz", d.ContactBID)
	}
}

// A pair reachable by both email and phone is one row, scored by the stronger key.
func TestDetect_PairFoundByTwoKeysIsRecordedOnce(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)
	seedContact(contactRepo, "c1", "Ada", "L", "shared@example.com", "+1 555 0001")
	seedContact(contactRepo, "c2", "Ada", "L", "shared@example.com", "15550001")

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 1, res.Found)
	require.Len(t, dupRepo.records, 1)

	for _, d := range dupRepo.records {
		require.Equal(t, 1.0, d.Score, "the stronger key wins")
	}
}

// A person's second email address identifies them exactly as well as their first one. Before
// the child tables were read, two records whose only shared address was a secondary one were
// never even compared, while the list's subset check — which does read contact_emails — was
// meanwhile willing to call one of them a lossless subset of the other.
func TestDetect_PairsOnASecondaryEmail(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)

	// Different primary addresses: nothing on the contact row pairs these two.
	seedContact(contactRepo, "c1", "Ada", "Lovelace", "ada@work.example.com", "+1 555 0001")
	seedContact(contactRepo, "c2", "A.", "Lovelace", "ada@home.example.com", "+1 555 0002")

	// The shared address exists only as a child row, and in different letter case.
	seedChildEmail(contactRepo, "c1", "ada.lovelace@example.com")
	seedChildEmail(contactRepo, "c2", "Ada.Lovelace@EXAMPLE.com")

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 1, res.Found)
	require.Equal(t,
		[]string{"c1+c2 score=1.00 reasons=[email_match(ada.lovelace@example.com)]"},
		detectedPairs(t, dupRepo),
		"a shared secondary address is an email match like any other")
}

// The same for a second phone number, which scores 0.8 and therefore also carries the name
// reason a phone pair has always carried.
func TestDetect_PairsOnASecondPhone(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)

	seedContact(contactRepo, "c1", "Grace", "Hopper", "grace@example.com", "+1 555 0001")
	seedContact(contactRepo, "c2", "Grace", "Hopper", "g.hopper@example.com", "+1 555 0002")

	// Same number, written the two ways people write it.
	seedChildPhone(contactRepo, "c1", "+1 (555) 9000")
	seedChildPhone(contactRepo, "c2", "15559000")

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 1, res.Found)
	require.Equal(t,
		[]string{"c1+c2 score=0.80 reasons=[name_exact(Grace Hopper) phone_match(15559000)]"},
		detectedPairs(t, dupRepo))
}

// Migration 014 is CREATE TABLE only — constitution Principle I forbids the backfill UPDATE
// that would have populated the child tables for contacts stored before it. A contact written
// then still has its `email` / `phone` columns and no child rows at all until something
// re-saves it, so the child values are folded INTO the existing buckets, never substituted
// for them.
func TestDetect_StillPairsAContactWithNoChildRows(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)

	// Two pre-014 rows: flat columns only.
	seedContact(contactRepo, "old1", "Ada", "Lovelace", "ada@example.com", "")
	seedContact(contactRepo, "old2", "Ada", "L.", "ADA@example.com", "")

	// A contact saved after 014, which has both, so the fixture is not accidentally a
	// book with no child rows in it.
	seedContact(contactRepo, "new1", "Grace", "Hopper", "grace@example.com", "")
	seedChildEmail(contactRepo, "new1", "grace@example.com")

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, 1, res.Found)
	require.Equal(t,
		[]string{"old1+old2 score=1.00 reasons=[email_match(ada@example.com)]"},
		detectedPairs(t, dupRepo))
}

// A contact whose primary phone is ALSO stored as a child row — which is what every contact
// saved through the normal write path looks like — must appear in that bucket once.
//
// Pair count would not catch a second entry: `a.ID == b.ID` skips the self-pair and `best`
// dedups by pair anyway. What a duplicate entry does is inflate len(bucket) against
// maxBucketSize, so a bucket that is scannable can be pushed over the cap and SKIPPED
// entirely — silently, from the user's point of view. Hence the size chosen here: 300
// contacts is comfortably under the cap of 500, and 600 is comfortably over it.
func TestDetect_DoesNotInflateABucketWhenThePrimaryAlsoAppearsAsAChildRow(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)

	const n = 300
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("c%04d", i)
		seedContact(contactRepo, id, "Person", fmt.Sprintf("%d", i), "", "+1 555 0000")
		seedChildPhone(contactRepo, id, "15550000")
	}

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, n*(n-1)/2, res.Found,
		"the bucket holds each contact once, so it stays under the cap and is still scanned")
	require.Len(t, dupRepo.records, n*(n-1)/2)
}

// One value shared by hundreds of contacts — a switchboard number, an info@ address — says
// nothing about identity, and comparing them all recreates the quadratic cost inside a bucket.
//
// The cap counts child values too: a switchboard number reaches the cap whether it sits in the
// `phone` column or in contact_phones, and 600 contacts holding it in the child table is the
// same non-identifying value as 600 holding it on the row.
func TestDetect_SkipsImplausiblyLargeBuckets(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)
	for i := 0; i < 600; i++ {
		seedContact(contactRepo, fmt.Sprintf("c%04d", i), "Person", fmt.Sprintf("%d", i), "", "+15550000")
	}

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Zero(t, res.Found, "an oversized bucket is skipped rather than exploded")
	require.Empty(t, dupRepo.records)
}

func TestDetect_SkipsImplausiblyLargeBucketsBuiltFromChildValues(t *testing.T) {
	det, contactRepo, dupRepo := newDetector(t)
	for i := 0; i < 600; i++ {
		id := fmt.Sprintf("c%04d", i)
		// No shared value on the contact row at all: the switchboard number is a child row.
		seedContact(contactRepo, id, "Person", fmt.Sprintf("%d", i), "", fmt.Sprintf("+1555%06d", i))
		seedChildPhone(contactRepo, id, "+1 555 0000")
	}

	res, err := det.Detect(context.Background(), "u1")
	require.NoError(t, err)
	require.Zero(t, res.Found, "a non-identifying value is non-identifying wherever it is stored")
	require.Empty(t, dupRepo.records)
}

// A cron scan and a manual one would both walk the whole book and race to insert the same
// pairs; the second is refused.
func TestDetect_RefusesAConcurrentRunForTheSameUser(t *testing.T) {
	det, contactRepo, _ := newDetector(t)
	seedContact(contactRepo, "c1", "Ada", "L", "shared@example.com", "")
	seedContact(contactRepo, "c2", "Ada", "L", "shared@example.com", "")

	release := make(chan struct{})
	started := make(chan struct{})
	contactRepo.beforeListForDedup = func() {
		close(started)
		<-release
	}

	done := make(chan error, 1)
	go func() {
		_, err := det.Detect(context.Background(), "u1")
		done <- err
	}()
	<-started

	_, err := det.Detect(context.Background(), "u1")
	require.ErrorIs(t, err, service.ErrDetectionInProgress)

	close(release)
	require.NoError(t, <-done)
}

// Cancelling the request stops the scan instead of running it to completion for a caller that
// has gone away.
func TestDetect_HonoursContextCancellation(t *testing.T) {
	det, contactRepo, _ := newDetector(t)
	for i := 0; i < 50; i++ {
		seedContact(contactRepo, fmt.Sprintf("c%02d", i), "P", fmt.Sprintf("%d", i),
			fmt.Sprintf("shared%d@example.com", i%2), "")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := det.Detect(ctx, "u1")
	require.ErrorIs(t, err, context.Canceled)
}
