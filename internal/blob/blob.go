// Package blob stores distribution files. The local implementation keeps them
// on disk; an S3-compatible one can be added behind the same interface.
package blob

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotFound = errors.New("blob not found")

// Object is an opened blob. ReadSeeker lets http.ServeContent handle ranges.
type Object struct {
	io.ReadSeekCloser
	Size    int64
	ModTime time.Time
}

type Store interface {
	// Put stores the content under key, replacing any existing object.
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (*Object, error)
	Delete(ctx context.Context, key string) error
	// SignedURL returns a short-lived direct URL, or "" when the backend
	// cannot produce one and the server should stream the object itself.
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	Close() error
}
