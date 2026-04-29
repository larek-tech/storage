package kafka

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Cfg содержит конфигурацию подключения к Kafka.
type Cfg struct {
	Brokers  []string `yaml:"brokers" env:"KAFKA_BROKERS"`
	GroupID  string   `yaml:"group_id" env:"KAFKA_GROUP_ID"`
	ClientID string   `yaml:"client_id" env:"KAFKA_CLIENT_ID"`
	User     string   `yaml:"user" env:"KAFKA_USER"`
	Password string   `yaml:"password" env:"KAFKA_PASSWORD"`
	UseTLS   bool     `yaml:"use_tls" env:"KAFKA_USE_TLS"`
	SASL     string   `yaml:"sasl" env:"KAFKA_SASL"` // "", "plain", "scram-sha-256", "scram-sha-512"
}

// DSN формирует строку подключения вида
// kafka://user:password@b1:9092,b2:9092/?group_id=...&sasl=plain&tls=true
func (cfg Cfg) DSN() string {
	scheme := "kafka"
	auth := ""
	if cfg.User != "" || cfg.Password != "" {
		auth = fmt.Sprintf("%s:%s@", cfg.User, cfg.Password)
	}
	hosts := strings.Join(cfg.Brokers, ",")

	q := url.Values{}
	if cfg.GroupID != "" {
		q.Set("group_id", cfg.GroupID)
	}
	if cfg.ClientID != "" {
		q.Set("client_id", cfg.ClientID)
	}
	if cfg.SASL != "" {
		q.Set("sasl", cfg.SASL)
	}
	if cfg.UseTLS {
		q.Set("tls", "true")
	}

	out := fmt.Sprintf("%s://%s%s", scheme, auth, hosts)
	if encoded := q.Encode(); encoded != "" {
		out += "/?" + encoded
	}
	return out
}

// NewCfgFromDSN парсит строку подключения вида
// "kafka://user:password@b1:9092,b2:9092/?group_id=...&sasl=plain&tls=true" в Cfg.
func NewCfgFromDSN(dsn string) (Cfg, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return Cfg{}, fmt.Errorf("parse dsn: %w", err)
	}
	if u.Scheme != "kafka" {
		return Cfg{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	cfg := Cfg{}
	if u.User != nil {
		cfg.User = u.User.Username()
		if pwd, ok := u.User.Password(); ok {
			cfg.Password = pwd
		}
	}

	hosts := u.Host
	if hosts == "" {
		return Cfg{}, fmt.Errorf("no brokers in dsn")
	}
	for raw := range strings.SplitSeq(hosts, ",") {
		broker, err := normalizeBroker(raw)
		if err != nil {
			return Cfg{}, err
		}
		cfg.Brokers = append(cfg.Brokers, broker)
	}

	q := u.Query()
	cfg.GroupID = q.Get("group_id")
	cfg.ClientID = q.Get("client_id")
	cfg.SASL = q.Get("sasl")
	if tls := q.Get("tls"); tls != "" {
		v, err := strconv.ParseBool(tls)
		if err != nil {
			return Cfg{}, fmt.Errorf("parse tls: %w", err)
		}
		cfg.UseTLS = v
	}
	return cfg, nil
}

func normalizeBroker(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty broker")
	}
	if !strings.Contains(raw, ":") {
		return "", fmt.Errorf("broker %q must include port", raw)
	}
	return raw, nil
}
