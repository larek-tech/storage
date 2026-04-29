package redis

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Cfg содержит конфигурацию подключения к Redis / Valkey.
type Cfg struct {
	User     string `yaml:"user" env:"REDIS_USER"`
	Password string `yaml:"password" env:"REDIS_PASSWORD"`
	Host     string `yaml:"host" env:"REDIS_HOST"`
	Port     int    `yaml:"port" env:"REDIS_PORT"`
	DB       int    `yaml:"db" env:"REDIS_DB"`
	UseTLS   bool   `yaml:"use_tls" env:"REDIS_USE_TLS"`
}

// DSN формирует строку подключения вида redis://user:password@host:port/db.
func (cfg Cfg) DSN() string {
	scheme := "redis"
	if cfg.UseTLS {
		scheme = "rediss"
	}
	auth := ""
	if cfg.User != "" || cfg.Password != "" {
		auth = fmt.Sprintf("%s:%s@", cfg.User, cfg.Password)
	}
	return fmt.Sprintf("%s://%s%s:%d/%d", scheme, auth, cfg.Host, cfg.Port, cfg.DB)
}

// NewCfgFromDSN парсит строку подключения вида
// "redis://user:password@host:port/db" (или rediss://) в Cfg.
func NewCfgFromDSN(dsn string) (Cfg, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return Cfg{}, fmt.Errorf("parse dsn: %w", err)
	}
	switch u.Scheme {
	case "redis":
	case "rediss":
	default:
		return Cfg{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	cfg := Cfg{
		Host:   u.Hostname(),
		UseTLS: u.Scheme == "rediss",
	}
	if u.User != nil {
		cfg.User = u.User.Username()
		if pwd, ok := u.User.Password(); ok {
			cfg.Password = pwd
		}
	}
	if portStr := u.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return Cfg{}, fmt.Errorf("parse port: %w", err)
		}
		cfg.Port = port
	}
	if dbStr := strings.TrimPrefix(u.Path, "/"); dbStr != "" {
		db, err := strconv.Atoi(dbStr)
		if err != nil {
			return Cfg{}, fmt.Errorf("parse db: %w", err)
		}
		cfg.DB = db
	}
	return cfg, nil
}
