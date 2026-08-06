package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

const duplicateScoreThreshold = 0.8

// maxBucketSize caps how many contacts sharing one key are compared against each other.
//
// A single "magnet" value — a shared office number, an info@ address on fifty records —
// would otherwise recreate the quadratic blow-up inside one bucket. Such a key says nothing
// about identity anyway.
const maxBucketSize = 500

// ErrDetectionInProgress reports a scan already running for the same user.
var ErrDetectionInProgress = errors.New("duplicate detection is already running")

// DuplicateDetector scans a user's address book for likely duplicate contacts.
type DuplicateDetector struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
	dupRepo     repository.PotentialDuplicateRepository
	logger      *zap.Logger

	// running guards against a scheduled scan and a manual one overlapping: both would walk
	// the whole address book and race to insert the same pairs.
	mu      sync.Mutex
	running map[string]bool
}

func NewDuplicateDetector(
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
	dupRepo repository.PotentialDuplicateRepository,
	logger *zap.Logger,
) *DuplicateDetector {
	return &DuplicateDetector{
		contactRepo: contactRepo,
		abRepo:      abRepo,
		dupRepo:     dupRepo,
		logger:      logger,
		running:     make(map[string]bool),
	}
}

type DetectionResult struct {
	Found   int `json:"found"`
	Checked int `json:"checked"`
}

// MatchReason explains why two contacts were paired.
//
// The older format was a bare []string ("email_match"), which told the UI what kind of match
// it was but not what actually matched — and recomputing that on the client is wrong as soon
// as a contact has more than one phone number. Existing rows are left in the old shape; the
// reader has to accept both.
type MatchReason struct {
	Code  string `json:"code"`
	Value string `json:"value,omitempty"`
}

// candidate is the slim projection the scan works on.
type candidate struct {
	contact *domain.Contact
	name    string
}

// Detect groups contacts by the keys that can actually produce a match and compares only
// within those groups.
//
// This is equivalent to the previous all-pairs scan, and the reason is worth writing down:
// the 0.8 threshold was reachable only two ways — an exact email (scored 1.0, returned
// immediately) or a normalised phone match (0.8). Name similarity scored 0.7 at best and was
// always discarded by the threshold, yet a Levenshtein distance was computed, with an
// allocation, for every pair in the book. Grouping by normalised email and phone therefore
// produces exactly the same pairs for a fraction of the work.
//
// Names still matter for the *reasons* attached to a pair — a phone match also records
// name_exact or name_similar — so they are compared inside a bucket, where it is cheap.
func (d *DuplicateDetector) Detect(ctx context.Context, userID string) (*DetectionResult, error) {
	if !d.begin(userID) {
		return nil, ErrDetectionInProgress
	}
	defer d.end(userID)

	ab, err := d.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	contacts, err := d.contactRepo.ListForDedup(ctx, ab.ID)
	if err != nil {
		return nil, err
	}

	result := &DetectionResult{Checked: len(contacts)}
	now := time.Now()

	// key → contacts sharing it. "email:" and "phone:" are kept apart so an address that
	// happens to look like a number cannot collide with one.
	buckets := map[string][]candidate{}
	for _, c := range contacts {
		cand := candidate{contact: c, name: strings.TrimSpace(c.FirstName + " " + c.LastName)}
		if email := strings.ToLower(strings.TrimSpace(c.Email)); email != "" {
			buckets["email:"+email] = append(buckets["email:"+email], cand)
		}
		if phone := normalizePhone(c.Phone); phone != "" {
			buckets["phone:"+phone] = append(buckets["phone:"+phone], cand)
		}
	}

	// A pair reachable by both email and phone must be recorded once, with the stronger score.
	type pairKey struct{ a, b string }
	best := map[pairKey]*domain.PotentialDuplicate{}

	for key, bucket := range buckets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(bucket) < 2 {
			continue
		}
		if len(bucket) > maxBucketSize {
			d.logger.Warn("skipping an implausibly large duplicate bucket",
				zap.String("key", redactKey(key)),
				zap.Int("size", len(bucket)),
				zap.String("reason", "a value shared by this many contacts does not identify a person"))
			continue
		}

		kind, value, _ := strings.Cut(key, ":")
		for i := 0; i < len(bucket); i++ {
			for j := i + 1; j < len(bucket); j++ {
				a, b := bucket[i].contact, bucket[j].contact
				if a.ID == b.ID {
					continue
				}

				score, reasons := scoreBucketPair(kind, value, bucket[i], bucket[j])
				if score < duplicateScoreThreshold {
					continue
				}

				// Normalised order, so the same pair found through two different keys is one
				// row and the unique index in migration 024 can do its job.
				aID, bID := a.ID, b.ID
				if aID > bID {
					aID, bID = bID, aID
				}

				pk := pairKey{aID, bID}
				if existing, ok := best[pk]; ok && existing.Score >= score {
					continue
				}

				encoded, _ := json.Marshal(reasons)
				best[pk] = &domain.PotentialDuplicate{
					ID:           uuid.New().String(),
					UserID:       userID,
					ContactAID:   aID,
					ContactBID:   bID,
					Score:        score,
					MatchReasons: string(encoded),
					Status:       "pending",
					CreatedAt:    now,
				}
			}
		}
	}

	for _, dup := range best {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Insert-or-ignore rather than select-then-insert: two overlapping scans would
		// otherwise both see "not present" and both insert.
		created, err := d.dupRepo.CreateIfAbsent(ctx, dup)
		if err != nil {
			d.logger.Warn("failed to record a duplicate pair", zap.Error(err))
			continue
		}
		if created {
			result.Found++
		}
	}

	return result, nil
}

// begin claims the scan slot for a user, returning false if one is already running.
func (d *DuplicateDetector) begin(userID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running[userID] {
		return false
	}
	d.running[userID] = true
	return true
}

func (d *DuplicateDetector) end(userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.running, userID)
}

// redactKey keeps a bucket warning useful without writing an address into the log.
func redactKey(key string) string {
	kind, value, found := strings.Cut(key, ":")
	if !found {
		return "unknown"
	}
	if len(value) > 4 {
		value = value[:4] + "…"
	}
	return kind + ":" + value
}

// scoreBucketPair scores two contacts that already share a key.
//
// The key is the match, so the score follows from its kind. Name comparison adds nothing to
// the score — it never could reach the threshold on its own — but it is what the reasons
// carry, and the old all-pairs scorer appended it to phone matches too.
func scoreBucketPair(kind, value string, a, b candidate) (float64, []MatchReason) {
	var reasons []MatchReason
	var score float64

	switch kind {
	case "email":
		// An exact email match returned immediately in the old scorer, before names were
		// looked at, so an email pair carries exactly one reason. Reproduced deliberately:
		// the characterisation test pins it, and adding a name reason here would change what
		// every existing pair means.
		return 1.0, []MatchReason{{Code: "email_match", Value: value}}
	case "phone":
		score = 0.8
		reasons = append(reasons, MatchReason{Code: "phone_match", Value: value})
	default:
		return 0, nil
	}

	if a.name != "" && b.name != "" {
		switch {
		case strings.EqualFold(a.name, b.name):
			reasons = append(reasons, MatchReason{Code: "name_exact", Value: a.name})
		case levenshtein(strings.ToLower(a.name), strings.ToLower(b.name)) <= 2:
			reasons = append(reasons, MatchReason{Code: "name_similar", Value: a.name})
		}
	}

	return score, reasons
}

// scoreContacts is the previous all-pairs scorer, kept for the characterisation test that
// pins what the bucketed scan must reproduce.
func scoreContacts(a, b *domain.Contact) (float64, []string) {
	// Exact email → immediate max score
	if a.Email != "" && strings.EqualFold(a.Email, b.Email) {
		return 1.0, []string{"email_match"}
	}

	var reasons []string
	score := 0.0

	// Normalised phone match
	pa, pb := normalizePhone(a.Phone), normalizePhone(b.Phone)
	if pa != "" && pa == pb {
		score = 0.8
		reasons = append(reasons, "phone_match")
	}

	// Full name
	nameA := strings.TrimSpace(a.FirstName + " " + a.LastName)
	nameB := strings.TrimSpace(b.FirstName + " " + b.LastName)
	if nameA != "" && nameB != "" {
		if strings.EqualFold(nameA, nameB) {
			if 0.7 > score {
				score = 0.7
			}
			reasons = append(reasons, "name_exact")
		} else if levenshtein(strings.ToLower(nameA), strings.ToLower(nameB)) <= 2 {
			if 0.5 > score {
				score = 0.5
			}
			reasons = append(reasons, "name_similar")
		}
	}

	return score, reasons
}

var _ = fmt.Sprintf // keep fmt available for future diagnostics

func normalizePhone(p string) string {
	if p == "" {
		return ""
	}
	var sb strings.Builder
	for _, ch := range p {
		if ch >= '0' && ch <= '9' {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// levenshtein computes the edit distance between two strings (no external deps).
//
// No longer on the hot path — it runs only inside a bucket, where the candidates already
// share an email or a phone number — but still the thing that decides name_similar.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	dp := make([]int, lb+1)
	for j := range dp {
		dp[j] = j
	}
	for i := 1; i <= la; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= lb; j++ {
			tmp := dp[j]
			if ra[i-1] == rb[j-1] {
				dp[j] = prev
			} else {
				dp[j] = 1 + minInt(prev, minInt(dp[j], dp[j-1]))
			}
			prev = tmp
		}
	}
	return dp[lb]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
