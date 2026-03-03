package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/contactshq/internal/repository"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	"go.uber.org/zap"
)

type SyncJobPayload struct {
	UserID       string `json:"user_id"`
	ProviderType string `json:"provider_type"`
	Config       string `json:"config"`
}

type cardDAVSyncConfig struct {
	CredentialID  string `json:"credential_id,omitempty"` // reference to stored credential
	Endpoint      string `json:"endpoint"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
}

type SyncJobHandler struct {
	engine      *chqsync.Engine
	contactRepo repository.ContactRepository
	abRepo      repository.AddressBookRepository
	credRepo    repository.ProviderConnectionRepository // optional: resolves credential_id
	logger      *zap.Logger
}

func NewSyncJobHandler(
	engine *chqsync.Engine,
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
	credRepo repository.ProviderConnectionRepository,
	logger *zap.Logger,
) *SyncJobHandler {
	return &SyncJobHandler{engine: engine, contactRepo: contactRepo, abRepo: abRepo, credRepo: credRepo, logger: logger}
}

func (h *SyncJobHandler) Handle(ctx context.Context, payload json.RawMessage) error {
	var p SyncJobPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshal sync job payload: %w", err)
	}

	h.logger.Info("sync job", zap.String("user_id", p.UserID), zap.String("provider", p.ProviderType))

	internal := chqsync.NewInternalProvider(h.contactRepo, h.abRepo, p.UserID)

	switch p.ProviderType {
	case "carddav":
		var cfg cardDAVSyncConfig
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return fmt.Errorf("unmarshal carddav config: %w", err)
		}
		// Resolve credential_id reference if present
		if cfg.CredentialID != "" && h.credRepo != nil {
			cred, err := h.credRepo.GetByID(ctx, cfg.CredentialID)
			if err != nil || cred == nil {
				return fmt.Errorf("credential %s not found", cfg.CredentialID)
			}
			cfg.Endpoint = cred.Endpoint
			cfg.Username = cred.Username
			cfg.Password = cred.Password
			cfg.SkipTLSVerify = cred.SkipTLSVerify
		}
		remote, err := chqsync.NewCardDAVClientProviderWithOptions(cfg.Endpoint, cfg.Username, cfg.Password, cfg.SkipTLSVerify)
		if err != nil {
			return fmt.Errorf("create carddav provider: %w", err)
		}
		result, err := h.engine.Sync(ctx, p.UserID, "", remote, internal, chqsync.ConflictSourceWins)
		if err != nil {
			return fmt.Errorf("carddav sync failed: %w", err)
		}
		h.logger.Info("carddav sync complete",
			zap.String("user_id", p.UserID),
			zap.Int("created", result.Created),
			zap.Int("updated", result.Updated),
			zap.Int("deleted", result.Deleted),
			zap.Int("errors", result.Errors),
		)
		return nil
	default:
		return fmt.Errorf("unsupported provider type: %s", p.ProviderType)
	}
}
