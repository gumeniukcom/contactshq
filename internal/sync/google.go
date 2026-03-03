package sync

import (
	"context"
	"fmt"
)

// GoogleProvider is a placeholder for Google People API sync.
// Full implementation requires OAuth2 and Google People API client.
type GoogleProvider struct {
	accessToken string
}

func NewGoogleProvider(accessToken string) *GoogleProvider {
	return &GoogleProvider{accessToken: accessToken}
}

func (p *GoogleProvider) Name() string {
	return "google"
}

func (p *GoogleProvider) List(ctx context.Context) ([]SyncItem, error) {
	return nil, fmt.Errorf("google sync not yet implemented: requires OAuth2 setup")
}

func (p *GoogleProvider) Get(ctx context.Context, remoteID string) (*SyncItem, error) {
	return nil, fmt.Errorf("google sync not yet implemented")
}

func (p *GoogleProvider) Put(ctx context.Context, item SyncItem) (string, error) {
	return "", fmt.Errorf("google sync not yet implemented")
}

func (p *GoogleProvider) Delete(ctx context.Context, remoteID string) error {
	return fmt.Errorf("google sync not yet implemented")
}
