package artifacts

import (
	"context"
	"io"
)

// Store persists and deletes binary artifact objects.
type Store interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string, contentLength int64) error
	Delete(ctx context.Context, key string) error
}
