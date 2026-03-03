package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

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
	v.SetDefault("auth.jwt_secret", "change-me-in-production")
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

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
