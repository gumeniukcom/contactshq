package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"go.uber.org/zap"
)

// OAuthHTTPClientProvider returns an authenticated HTTP client for a provider connection.
// Implemented by service.GoogleOAuthService.
type OAuthHTTPClientProvider interface {
	GetHTTPClient(ctx context.Context, conn *domain.ProviderConnection) (*http.Client, error)
}

type PipelineOrchestrator struct {
	engine       *Engine
	contactRepo  repository.ContactRepository
	abRepo       repository.AddressBookRepository
	pipelineRepo repository.PipelineRepository
	credRepo     repository.ProviderConnectionRepository // optional: resolves credential_id refs
	googleOAuth  OAuthHTTPClientProvider                 // optional: for Google provider
	logger       *zap.Logger
}

func NewPipelineOrchestrator(
	engine *Engine,
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
	pipelineRepo repository.PipelineRepository,
	credRepo repository.ProviderConnectionRepository,
	googleOAuth OAuthHTTPClientProvider,
	logger *zap.Logger,
) *PipelineOrchestrator {
	return &PipelineOrchestrator{
		engine:       engine,
		contactRepo:  contactRepo,
		abRepo:       abRepo,
		pipelineRepo: pipelineRepo,
		credRepo:     credRepo,
		googleOAuth:  googleOAuth,
		logger:       logger,
	}
}

type StepResult struct {
	StepOrder int         `json:"step_order"`
	Result    *SyncResult `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
}

func (o *PipelineOrchestrator) Execute(ctx context.Context, userID string, pipeline *domain.Pipeline) ([]StepResult, error) {
	steps, err := o.pipelineRepo.GetSteps(ctx, pipeline.ID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}

	results := make([]StepResult, 0, len(steps))

	for _, step := range steps {
		source, err := o.createProvider(ctx, userID, step.SourceType, step.SourceConfig)
		if err != nil {
			results = append(results, StepResult{
				StepOrder: step.Order,
				Error:     fmt.Sprintf("create source provider: %v", err),
			})
			continue
		}

		dest, err := o.createProvider(ctx, userID, step.DestType, step.DestConfig)
		if err != nil {
			results = append(results, StepResult{
				StepOrder: step.Order,
				Error:     fmt.Sprintf("create dest provider: %v", err),
			})
			continue
		}

		conflictMode := ConflictMode(step.ConflictMode)
		mode := SyncMode(step.Direction)
		if mode == "" {
			mode = SyncModePull
		}
		result, err := o.engine.Sync(ctx, userID, pipeline.ID, source, dest, conflictMode, mode)
		if err != nil {
			results = append(results, StepResult{
				StepOrder: step.Order,
				Error:     fmt.Sprintf("sync: %v", err),
			})
			continue
		}

		o.logger.Info("pipeline step completed",
			zap.String("pipeline_id", pipeline.ID),
			zap.Int("step", step.Order),
			zap.Int("created", result.Created),
			zap.Int("updated", result.Updated),
			zap.Int("deleted", result.Deleted),
		)

		results = append(results, StepResult{
			StepOrder: step.Order,
			Result:    result,
		})
	}

	return results, nil
}

type providerConfig struct {
	CredentialID  string `json:"credential_id,omitempty"` // reference to a stored credential
	Endpoint      string `json:"endpoint"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	AccessToken   string `json:"access_token"`
}

func (o *PipelineOrchestrator) createProvider(ctx context.Context, userID, providerType, configJSON string) (SyncProvider, error) {
	switch providerType {
	case "internal":
		return NewInternalProvider(o.contactRepo, o.abRepo, userID), nil

	case "carddav":
		var cfg providerConfig
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return nil, fmt.Errorf("parse carddav config: %w", err)
		}
		if cfg.CredentialID != "" && o.credRepo != nil {
			cred, err := o.credRepo.GetByID(ctx, cfg.CredentialID)
			if err != nil || cred == nil {
				return nil, fmt.Errorf("credential %s not found", cfg.CredentialID)
			}
			// OAuth2 credential → use authenticated HTTP client for CardDAV
			if cred.AccessToken != "" && o.googleOAuth != nil {
				httpClient, err := o.googleOAuth.GetHTTPClient(ctx, cred)
				if err != nil {
					return nil, fmt.Errorf("carddav oauth http client: %w", err)
				}
				return NewCardDAVClientProviderWithHTTPClient(cred.Endpoint, httpClient)
			}
			cfg.Endpoint = cred.Endpoint
			cfg.Username = cred.Username
			cfg.Password = cred.Password
			cfg.SkipTLSVerify = cred.SkipTLSVerify
		}
		return NewCardDAVClientProviderWithOptions(cfg.Endpoint, cfg.Username, cfg.Password, cfg.SkipTLSVerify)

	case "google":
		var cfg providerConfig
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return nil, fmt.Errorf("parse google config: %w", err)
		}
		if o.googleOAuth == nil {
			return nil, fmt.Errorf("google oauth service not configured")
		}
		if cfg.CredentialID == "" {
			return nil, fmt.Errorf("google provider requires credential_id")
		}
		conn, err := o.credRepo.GetByID(ctx, cfg.CredentialID)
		if err != nil || conn == nil {
			return nil, fmt.Errorf("google credential %s not found", cfg.CredentialID)
		}
		httpClient, err := o.googleOAuth.GetHTTPClient(ctx, conn)
		if err != nil {
			return nil, fmt.Errorf("google http client: %w", err)
		}
		return NewGoogleProviderWithClient(ctx, httpClient, o.logger)

	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}
