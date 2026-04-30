package s3

import (
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage описывает минимальный контракт объектного хранилища.
type Storage interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentType string) error
	GetObject(ctx context.Context, key string) (*s3.GetObjectOutput, error)
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)
	PresignPutObject(ctx context.Context, key string, expiry time.Duration) (string, error)
	Bucket() string
}
