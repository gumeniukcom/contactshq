package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
	"go.uber.org/zap"
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

func (p *GoogleProvider) List(ctx context.Context) ([]SyncItem, error) {
	var items []SyncItem
	var pageToken string

	for {
		call := p.service.People.Connections.List("people/me").
			PersonFields(allPersonFields).
			PageSize(100).
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list contacts: %w", err)
		}

		for _, person := range resp.Connections {
			// Skip deleted contacts
			if person.Metadata != nil && person.Metadata.Deleted {
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
			items = append(items, SyncItem{
				RemoteID:    person.ResourceName,
				ETag:        person.Etag,
				ContentHash: hex.EncodeToString(h[:]),
				VCardData:   vcardData,
			})
		}

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return items, nil
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

func (p *GoogleProvider) Put(ctx context.Context, item SyncItem) (string, error) {
	parsed, err := vcardpkg.ParseVCard(item.VCardData)
	if err != nil {
		return "", fmt.Errorf("parse vcard: %w", err)
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
			return "", fmt.Errorf("update contact %s: %w", item.RemoteID, err)
		}
		return updated.Etag, nil
	}

	// Create new contact
	created, err := p.service.People.CreateContact(person).
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("create contact: %w", err)
	}
	return created.Etag, nil
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
