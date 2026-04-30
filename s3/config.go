package s3

import (
	"errors"
	"fmt"
	"net/url"
)

// Cfg содержит конфигурацию подключения к S3 / S3-compatible хранилищу.
type Cfg struct {
	Region          string `yaml:"region" env:"S3_REGION"`
	Bucket          string `yaml:"bucket" env:"S3_BUCKET"`
	Endpoint        string `yaml:"endpoint" env:"S3_ENDPOINT"`
	AccessKeyID     string `yaml:"access_key_id" env:"S3_ACCESS_KEY_ID"`
	SecretAccessKey string `yaml:"secret_access_key" env:"S3_SECRET_ACCESS_KEY"`
	UsePathStyle    bool   `yaml:"use_path_style" env:"S3_USE_PATH_STYLE"`
}

// Validate проверяет, что конфигурация содержит обязательные поля.
func (cfg Cfg) Validate() error {
	if cfg.Region == "" {
		return errors.New("region is required")
	}
	if cfg.Bucket == "" {
		return errors.New("bucket is required")
	}
	if (cfg.AccessKeyID == "") != (cfg.SecretAccessKey == "") {
		return errors.New("both access_key_id and secret_access_key must be set")
	}
	if cfg.Endpoint == "" {
		return nil
	}
	parsed, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("endpoint must include scheme and host")
	}
	return nil
}
