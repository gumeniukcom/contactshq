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
	"google.client_id",
	"google.client_secret",
	"google.redirect_url",
	"server.trusted_proxies",
	"carddav.path_prefix",
	"backup.dir",
	"backup.schedule",
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
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "contactshq.db")
	v.SetDefault("auth.token_ttl", "24h")
	v.SetDefault("auth.refresh_ttl", "720h")
	v.SetDefault("carddav.path_prefix", "/dav")
	v.SetDefault("backup.dir", "./backups")
	v.SetDefault("backup.schedule", "0 2 * * *")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

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

// Validate rejects configurations that are unsafe to serve traffic with.
func (c *Config) Validate() error {
	if err := c.Auth.validate(); err != nil {
		return err
	}
	return c.Server.validate()
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
