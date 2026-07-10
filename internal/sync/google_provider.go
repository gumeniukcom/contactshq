package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
	"go.uber.org/zap"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

// GoogleProvider implements SyncProvider using the Google People API.
type GoogleProvider struct {
	service *people.Service
	logger  *zap.Logger
}

// NewGoogleProviderWithClient creates a GoogleProvider from an authenticated HTTP client.
func NewGoogleProviderWithClient(ctx context.Context, httpClient *http.Client, logger *zap.Logger) (*GoogleProvider, error) {
	srv, err := people.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create people service: %w", err)
	}
	return &GoogleProvider{service: srv, logger: logger}, nil
}

func (p *GoogleProvider) Name() string { return "google" }

var (
	_ IncrementalProvider = (*GoogleProvider)(nil)
	_ ConditionalWriter   = (*GoogleProvider)(nil)
)

func (p *GoogleProvider) List(ctx context.Context) ([]SyncItem, error) {
	delta, err := p.listConnections(ctx, "")
	if err != nil {
		return nil, err
	}
	return delta.Updated, nil
}

// ListChanges fetches only what the People API changed since cursor, using a sync token.
//
// An empty cursor requests the whole collection and a fresh sync token; a stored token
// requests just the changes since. Google reports removals inline as persons flagged
// metadata.deleted, which List used to drop on the floor — those are exactly the
// deletions incremental sync needs.
func (p *GoogleProvider) ListChanges(ctx context.Context, cursor string) (Delta, error) {
	return p.listConnections(ctx, cursor)
}

func (p *GoogleProvider) listConnections(ctx context.Context, syncTokenIn string) (Delta, error) {
	delta := Delta{Full: syncTokenIn == ""}

	var pageToken string
	for {
		call := p.service.People.Connections.List("people/me").
			PersonFields(allPersonFields).
			PageSize(100).
			RequestSyncToken(true).
			Context(ctx)

		if syncTokenIn != "" {
			call = call.SyncToken(syncTokenIn)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			if isExpiredSyncToken(err) {
				// The token is too old for Google to serve a delta from.
				return Delta{}, ErrCursorExpired
			}
			return Delta{}, fmt.Errorf("list contacts: %w", err)
		}

		for _, person := range resp.Connections {
			if person.Metadata != nil && person.Metadata.Deleted {
				delta.Deleted = append(delta.Deleted, person.ResourceName)
				continue
			}

			vcardData, err := PersonToVCard(person)
			if err != nil {
				p.logger.Warn("failed to convert person to vcard",
					zap.String("resource", person.ResourceName),
					zap.Error(err),
				)
				continue
			}

			h := sha256.Sum256([]byte(vcardData))
			delta.Updated = append(delta.Updated, SyncItem{
				RemoteID:    person.ResourceName,
				ETag:        person.Etag,
				ContentHash: hex.EncodeToString(h[:]),
				VCardData:   vcardData,
			})
		}

		pageToken = resp.NextPageToken
		if pageToken == "" {
			delta.Cursor = resp.NextSyncToken
			break
		}
	}

	return delta, nil
}

// isExpiredSyncToken recognises the 410 the People API returns for a sync token it can no
// longer honour. The delta since then is gone; the caller must re-list in full.
func isExpiredSyncToken(err error) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && gerr.Code == http.StatusGone
}

func (p *GoogleProvider) Get(ctx context.Context, remoteID string) (*SyncItem, error) {
	person, err := p.service.People.Get(remoteID).
		PersonFields(allPersonFields).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("get contact %s: %w", remoteID, err)
	}

	vcardData, err := PersonToVCard(person)
	if err != nil {
		return nil, fmt.Errorf("convert person to vcard: %w", err)
	}

	h := sha256.Sum256([]byte(vcardData))
	return &SyncItem{
		RemoteID:    person.ResourceName,
		ETag:        person.Etag,
		ContentHash: hex.EncodeToString(h[:]),
		VCardData:   vcardData,
	}, nil
}

func (p *GoogleProvider) Put(ctx context.Context, item SyncItem) (PutResult, error) {
	parsed, err := vcardpkg.ParseVCard(item.VCardData)
	if err != nil {
		return PutResult{}, fmt.Errorf("parse vcard: %w", err)
	}

	person := ParsedContactToPerson(parsed)

	if strings.HasPrefix(item.RemoteID, "people/") {
		// Update existing contact
		person.Etag = item.ETag
		updated, err := p.service.People.UpdateContact(item.RemoteID, person).
			UpdatePersonFields(allUpdatePersonFields).
			Context(ctx).
			Do()
		if err != nil {
			return PutResult{}, fmt.Errorf("update contact %s: %w", item.RemoteID, err)
		}
		remoteID := updated.ResourceName
		if remoteID == "" {
			remoteID = item.RemoteID
		}
		return PutResult{RemoteID: remoteID, ETag: updated.Etag}, nil
	}

	// Create new contact. People assigns the resourceName; without it we cannot match
	// this contact on the next sync.
	created, err := p.service.People.CreateContact(person).
		Context(ctx).
		Do()
	if err != nil {
		return PutResult{}, fmt.Errorf("create contact: %w", err)
	}
	if created.ResourceName == "" {
		return PutResult{}, fmt.Errorf("create contact: People API returned no resourceName")
	}
	return PutResult{RemoteID: created.ResourceName, ETag: created.Etag}, nil
}

// PutIfMatch updates a contact only if Google still holds the ETag we last saw. The
// People API enforces the etag on update and answers a mismatch with 400 FAILED_PRECONDITION,
// which becomes ErrPreconditionFailed. A create (empty ifMatch) has no prior version to
// guard, so it falls through to a normal Put.
func (p *GoogleProvider) PutIfMatch(ctx context.Context, item SyncItem, ifMatch string) (PutResult, error) {
	if ifMatch != "" {
		item.ETag = ifMatch
	}
	res, err := p.Put(ctx, item)
	if isPreconditionFailure(err) {
		return PutResult{}, ErrPreconditionFailed
	}
	return res, err
}

// isPreconditionFailure recognises the etag-mismatch People returns on a stale update.
// It reports 412, and 400 whose status is FAILED_PRECONDITION.
func isPreconditionFailure(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	if gerr.Code == http.StatusPreconditionFailed {
		return true
	}
	return gerr.Code == http.StatusBadRequest && strings.Contains(gerr.Message, "FAILED_PRECONDITION")
}

func (p *GoogleProvider) Delete(ctx context.Context, remoteID string) error {
	_, err := p.service.People.DeleteContact(remoteID).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("delete contact %s: %w", remoteID, err)
	}
	return nil
}
