package config

import (
	"errors"
	"fmt"
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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate rejects configurations that are unsafe to serve traffic with.
func (c *Config) Validate() error {
	return c.Auth.validate()
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
