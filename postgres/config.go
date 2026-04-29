package postgres

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Cfg содержит конфигурацию подключения к базе данных.
type Cfg struct {
	User     string `yaml:"user" env:"POSTGRES_USER"`
	Password string `yaml:"password" env:"POSTGRES_PASSWORD"`
	Host     string `yaml:"host" env:"POSTGRES_HOST"`
	Port     int    `yaml:"port" env:"POSTGRES_PORT"`
	DB       string `yaml:"db" env:"POSTGRES_DB"`
}

func (cfg Cfg) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DB)
}

// NewCfgFromDSN парсит строку подключения вида
// "postgres://user:password@host:port/db?sslmode=disable" в Cfg.
func NewCfgFromDSN(dsn string) (Cfg, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return Cfg{}, fmt.Errorf("parse dsn: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return Cfg{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	cfg := Cfg{
		Host: u.Hostname(),
		DB:   strings.TrimPrefix(u.Path, "/"),
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
	return cfg, nil
}
