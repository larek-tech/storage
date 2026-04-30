package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName           = "github.com/larek-tech/storage/s3"
	defaultPresignExpiry = 15 * time.Minute
)

// ObjectInfo описывает краткую информацию об объекте.
type ObjectInfo struct {
	Key  string
	Size int64
}

// Client — обёртка над AWS S3 client с опциональной OpenTelemetry-трассировкой.
type Client struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
	tracer  trace.Tracer
	withTel bool
}

// Option настраивает Client.
type Option func(*Client)

// WithTelemetry включает OpenTelemetry трассировку.
func WithTelemetry(enabled bool) Option {
	return func(c *Client) {
		c.withTel = enabled
	}
}

// WithTracer позволяет задать пользовательский tracer.
func WithTracer(tracer trace.Tracer) Option {
	return func(c *Client) {
		c.tracer = tracer
	}
}

// New создаёт клиента S3 по конфигурации.
func New(ctx context.Context, cfg Cfg, opts ...Option) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		loadOptions = append(loadOptions,
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
			),
		)
	}
	if cfg.Endpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				if service != s3.ServiceID {
					return aws.Endpoint{}, &aws.EndpointNotFoundError{}
				}
				return aws.Endpoint{
					URL:               cfg.Endpoint,
					SigningRegion:     cfg.Region,
					HostnameImmutable: true,
				}, nil
			},
		)
		loadOptions = append(loadOptions, config.WithEndpointResolverWithOptions(resolver))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
	})

	s3Client := &Client{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.Bucket,
		tracer:  otel.Tracer(tracerName),
		withTel: false,
	}
	for _, opt := range opts {
		opt(s3Client)
	}

	return s3Client, nil
}

func (c *Client) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !c.withTel {
		return ctx, trace.SpanFromContext(ctx)
	}
	return c.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// Bucket возвращает bucket, с которым работает клиент.
func (c *Client) Bucket() string {
	return c.bucket
}

// GetClient возвращает нижележащий *s3.Client.
func (c *Client) GetClient() *s3.Client {
	return c.client
}

// PutObject сохраняет объект в S3.
func (c *Client) PutObject(ctx context.Context, key string, body io.Reader, contentType string) error {
	ctx, span := c.startSpan(ctx, "S3.PutObject",
		attribute.String("bucket", c.bucket),
		attribute.String("key", key),
	)
	defer span.End()

	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	_, err := c.client.PutObject(ctx, input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// GetObject получает объект из S3. Не забудьте закрыть Body.
func (c *Client) GetObject(ctx context.Context, key string) (*s3.GetObjectOutput, error) {
	ctx, span := c.startSpan(ctx, "S3.GetObject",
		attribute.String("bucket", c.bucket),
		attribute.String("key", key),
	)
	defer span.End()

	output, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return output, nil
}

// ListObjects возвращает список объектов по префиксу.
func (c *Client) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	ctx, span := c.startSpan(ctx, "S3.ListObjects",
		attribute.String("bucket", c.bucket),
		attribute.String("prefix", prefix),
	)
	defer span.End()

	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	})

	var items []ObjectInfo
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		for _, obj := range page.Contents {
			items = append(items, ObjectInfo{
				Key:  aws.ToString(obj.Key),
				Size: aws.ToInt64(obj.Size),
			})
		}
	}

	span.SetAttributes(attribute.Int("items_count", len(items)))
	return items, nil
}

// PresignPutObject генерирует ссылку для прямой загрузки объекта.
func (c *Client) PresignPutObject(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = defaultPresignExpiry
	}

	ctx, span := c.startSpan(ctx, "S3.PresignPutObject",
		attribute.String("bucket", c.bucket),
		attribute.String("key", key),
		attribute.Int64("expiry_ms", expiry.Milliseconds()),
	)
	defer span.End()

	presigned, err := c.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	return presigned.URL, nil
}
