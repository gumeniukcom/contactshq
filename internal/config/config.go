package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// MinJWTSecretLen is the shortest accepted HS256 signing secret.
const MinJWTSecretLen = 32

// weakJWTSecrets are placeholder values that have shipped in example configs and
// must never sign tokens on a real deployment.
var weakJWTSecrets = map[string]struct{}{
	"change-me-in-production": {},
	"changeme":                {},
	"secret":                  {},
	"development":             {},
	"test":                    {},
}

var ErrWeakJWTSecret = errors.New("insecure auth.jwt_secret")

// ErrInvalidTrustedProxy flags a server.trusted_proxies entry that is neither an IP nor a CIDR.
var ErrInvalidTrustedProxy = errors.New("invalid trusted proxy")

// envBoundKeys lists every config key overridable via a CHQ_-prefixed env var.
var envBoundKeys = []string{
	"server.port",
	"server.host",
	"database.driver",
	"database.dsn",
	"auth.jwt_secret",
	"auth.token_ttl",
	"auth.refresh_ttl",
	"auth.allow_registration",
	"google.client_id",
	"google.client_secret",
	"google.redirect_url",
	"server.trusted_proxies",
	"carddav.path_prefix",
	"backup.dir",
	"backup.schedule",
	"backup.max_restore_bytes",
	"merge.log_retention_days",
	"sync.runs_retention_days",
	"log.level",
	"log.format",
}

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Google   GoogleConfig   `mapstructure:"google"`
	CardDAV  CardDAVConfig  `mapstructure:"carddav"`
	Backup   BackupConfig   `mapstructure:"backup"`
	Merge    MergeConfig    `mapstructure:"merge"`
	Sync     SyncConfig     `mapstructure:"sync"`
	Log      LogConfig      `mapstructure:"log"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`

	// TrustedProxies lists the reverse proxies whose X-Forwarded-For header may be
	// believed, as IPs or CIDR ranges. Empty (the default) means the app trusts no
	// forwarded header and treats the direct peer as the client — safe when exposed
	// directly, but it collapses per-client rate limiting behind a proxy.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

type AuthConfig struct {
	JWTSecret  string        `mapstructure:"jwt_secret"`
	TokenTTL   time.Duration `mapstructure:"token_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`

	// AllowRegistration opens the public /auth/register endpoint to anyone who can reach the
	// port. It defaults to false: creating the first account is always permitted so a fresh
	// instance can be bootstrapped, but after that sign-up is an explicit choice.
	AllowRegistration bool `mapstructure:"allow_registration"`
}

type GoogleConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
}

type CardDAVConfig struct {
	PathPrefix string `mapstructure:"path_prefix"`
}

type BackupConfig struct {
	Dir      string `mapstructure:"dir"`
	Schedule string `mapstructure:"schedule"`

	// MaxRestoreBytes caps the decompressed size a restore will read. A gzip backup can
	// expand by orders of magnitude and restore holds the result in memory.
	MaxRestoreBytes int64 `mapstructure:"max_restore_bytes"`
}

// MergeConfig governs the record kept of contact merges.
type MergeConfig struct {
	// LogRetentionDays bounds how long merge_log rows live. Each row keeps a snapshot of the
	// discarded card, so without pruning the table grows by a contact per merge forever.
	LogRetentionDays int `mapstructure:"log_retention_days"`
}

// SyncConfig governs the record kept of pipeline runs.
type SyncConfig struct {
	// RunsRetentionDays bounds how long sync_runs rows live. The table gains a row per
	// pipeline execution, so unlike backup_runs it grows without bound.
	RunsRetentionDays int `mapstructure:"runs_retention_days"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// setDefaults registers every default in one place. Kept separate from Load so a test can
// assert that the set of known keys stays a subset of envBoundKeys: a key with a default but
// no BindEnv is invisible to AutomaticEnv, which is how CHQ_AUTH_JWT_SECRET was once ignored.
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "contactshq.db")
	v.SetDefault("auth.token_ttl", "24h")
	v.SetDefault("auth.refresh_ttl", "720h")
	v.SetDefault("auth.allow_registration", false)
	v.SetDefault("carddav.path_prefix", "/dav")
	v.SetDefault("backup.dir", "./backups")
	v.SetDefault("backup.schedule", "0 2 * * *")
	v.SetDefault("backup.max_restore_bytes", 128<<20)
	v.SetDefault("merge.log_retention_days", 30)
	v.SetDefault("sync.runs_retention_days", 90)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
}

// Load reads the configuration for serving traffic. It refuses anything the server must not
// start with — most importantly a missing or guessable auth.jwt_secret.
func Load() (*Config, error) {
	cfg, err := load()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadForCLI reads the same configuration for a subcommand that only touches the database.
//
// The full Validate is deliberately not relaxed for the server: refusing to start without a
// real signing secret is load-bearing. But a `set-password` run signs no tokens, and an
// operator recovering access to a deployment should not first have to reconstruct the secret
// the running server holds in its environment.
func LoadForCLI() (*Config, error) {
	cfg, err := load()
	if err != nil {
		return nil, err
	}
	if err := cfg.Database.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func load() (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	setDefaults(v)

	v.SetEnvPrefix("CHQ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// AutomaticEnv only reaches Unmarshal for keys viper already knows about, so a key
	// with neither a default nor a config-file entry is invisible to it. Bind every key
	// explicitly — otherwise CHQ_AUTH_JWT_SECRET and the Google settings are silently
	// ignored on env-only deployments such as docker compose.
	for _, key := range envBoundKeys {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("bind env for %s: %w", key, err)
		}
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// A list from an env var arrives as one comma-joined string; a YAML list arrives as
	// separate elements. Normalise both to a clean slice.
	cfg.Server.TrustedProxies = splitList(cfg.Server.TrustedProxies)

	return &cfg, nil
}

// splitList flattens comma-separated entries and drops blanks, so both a YAML list and a
// single "a,b,c" env value produce the same result.
func splitList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		for _, part := range strings.Split(item, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// ErrInvalidDatabase flags a database section a command cannot work with.
var ErrInvalidDatabase = errors.New("invalid database configuration")

// Validate rejects configurations that are unsafe to serve traffic with.
//
// Auth comes first on purpose: a weak signing secret is the one misconfiguration that lets
// anyone forge admin tokens, so it is the message an operator should see.
func (c *Config) Validate() error {
	if err := c.Auth.validate(); err != nil {
		return err
	}
	if err := c.Server.validate(); err != nil {
		return err
	}
	return c.Database.validate()
}

func (d DatabaseConfig) validate() error {
	switch d.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("%w: database.driver must be sqlite or postgres, got %q",
			ErrInvalidDatabase, d.Driver)
	}
	if strings.TrimSpace(d.DSN) == "" {
		return fmt.Errorf("%w: database.dsn is not set", ErrInvalidDatabase)
	}
	return nil
}

func (s ServerConfig) validate() error {
	// Fail fast on a typo rather than silently trusting nothing, which would leave the
	// operator believing rate limiting keys on the real client.
	for _, p := range s.TrustedProxies {
		if net.ParseIP(p) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(p); err == nil {
			continue
		}
		return fmt.Errorf("%w: %q is not an IP address or CIDR range", ErrInvalidTrustedProxy, p)
	}
	return nil
}

func (a AuthConfig) validate() error {
	secret := strings.TrimSpace(a.JWTSecret)

	if secret == "" {
		return fmt.Errorf("%w: auth.jwt_secret is not set — generate one with `openssl rand -hex 32` "+
			"and pass it via CHQ_AUTH_JWT_SECRET or configs/config.yaml", ErrWeakJWTSecret)
	}

	if _, weak := weakJWTSecrets[strings.ToLower(secret)]; weak {
		return fmt.Errorf("%w: auth.jwt_secret is a well-known placeholder value — anyone can forge "+
			"admin tokens against this server. Generate one with `openssl rand -hex 32`", ErrWeakJWTSecret)
	}

	if len(secret) < MinJWTSecretLen {
		return fmt.Errorf("%w: auth.jwt_secret must be at least %d characters (got %d) — "+
			"generate one with `openssl rand -hex 32`", ErrWeakJWTSecret, MinJWTSecretLen, len(secret))
	}

	return nil
}
