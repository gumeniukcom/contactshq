package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"go.uber.org/zap"
)

const duplicateScoreThreshold = 0.8

// DuplicateDetector scans a user's address book for likely duplicate contacts.
type DuplicateDetector struct {
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
	dupRepo     repository.PotentialDuplicateRepository
	logger      *zap.Logger
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
	}
}

type DetectionResult struct {
	Found   int `json:"found"`
	Checked int `json:"checked"`
}

// Detect runs O(N²) pairwise scoring over all contacts in the user's address book.
// Pairs above the threshold that are not yet tracked are stored in potential_duplicates.
func (d *DuplicateDetector) Detect(ctx context.Context, userID string) (*DetectionResult, error) {
	ab, err := d.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	contacts, err := d.contactRepo.ListAll(ctx, ab.ID)
	if err != nil {
		return nil, err
	}

	result := &DetectionResult{Checked: len(contacts)}
	now := time.Now()

	for i := 0; i < len(contacts); i++ {
		for j := i + 1; j < len(contacts); j++ {
			a, b := contacts[i], contacts[j]
			score, reasons := scoreContacts(a, b)
			if score < duplicateScoreThreshold {
				continue
			}

			// Skip if already tracked
			existing, err := d.dupRepo.GetByContacts(ctx, userID, a.ID, b.ID)
			if err != nil {
				d.logger.Warn("duplicate check error", zap.Error(err))
				continue
			}
			if existing != nil {
				continue
			}

			reasonsJSON, _ := json.Marshal(reasons)
			dup := &domain.PotentialDuplicate{
				ID:           uuid.New().String(),
				UserID:       userID,
				ContactAID:   a.ID,
				ContactBID:   b.ID,
				Score:        score,
				MatchReasons: string(reasonsJSON),
				Status:       "pending",
				CreatedAt:    now,
			}
			if err := d.dupRepo.Create(ctx, dup); err != nil {
				d.logger.Warn("failed to create duplicate record", zap.Error(err))
				continue
			}
			result.Found++
		}
	}

	return result, nil
}

// scoreContacts returns a similarity score [0,1] and the matching reasons.
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
