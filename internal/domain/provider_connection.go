package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// ProviderConnection stores credentials for a user's external sync provider.
// Passwords are stored in plaintext — this is a self-hosted app; add encryption
// when handling multi-tenant or cloud deployments.
type ProviderConnection struct {
	bun.BaseModel `bun:"table:provider_connections,alias:pc"`

	ID            string     `bun:",pk,type:text"                               json:"id"`
	UserID        string     `bun:",notnull"                                    json:"-"`
	ProviderType  string     `bun:",notnull"                                    json:"provider_type"`
	Name          string     `bun:",notnull,default:''"                         json:"name"`
	Endpoint      string     `bun:",notnull,default:''"                         json:"endpoint"`
	Username      string     `bun:",notnull,default:''"                         json:"username"`
	Password      string     `bun:",notnull,default:''"                         json:"-"` // never expose in API
	SkipTLSVerify bool       `bun:"skip_tls_verify,notnull,default:false"       json:"skip_tls_verify"`
	Connected     bool       `bun:",notnull,default:true"                       json:"connected"`
	LastSyncAt    *time.Time `bun:",nullzero"                                   json:"last_sync_at,omitempty"`
	LastError     string     `bun:",notnull,default:''"                         json:"last_error"`
	AccessToken   string     `bun:",notnull,default:''"                         json:"-"`
	RefreshToken  string     `bun:",notnull,default:''"                         json:"-"`
	TokenExpiry   *time.Time `bun:",nullzero"                                   json:"-"`
	ClientID      string     `bun:",notnull,default:''"                         json:"client_id,omitempty"`
	ClientSecret  string     `bun:",notnull,default:''"                         json:"-"`
	Scopes        string     `bun:",notnull,default:''"                         json:"scopes,omitempty"`
	CreatedAt     time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
