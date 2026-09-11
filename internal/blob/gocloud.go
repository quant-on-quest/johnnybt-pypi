package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob" // file://
	_ "gocloud.dev/blob/memblob"  // mem://  (tests)
	_ "gocloud.dev/blob/s3blob"   // s3://   (AWS, Aliyun OSS, Tencent COS, R2, MinIO…)
	"gocloud.dev/gcerrors"
)

// Bucket adapts a Go CDK bucket to Store, so the storage backend is chosen
// by URL: "s3://name?region=…", "file:///var/lib/pypi/blobs", "mem://".
// Adding GCS or Azure is one blank import away (gcsblob / azureblob).
type Bucket struct {
	b      *blob.Bucket
	signed bool
}

// OpenURL opens the bucket at rawURL. A "prefix=" query parameter scopes every
// key under that prefix (handy when sharing a bucket). signedURLs controls
// whether SignedURL hands out redirect targets.
func OpenURL(ctx context.Context, rawURL string, signedURLs bool) (*Bucket, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("blob url: %w", err)
	}
	q := u.Query()
	prefix := q.Get("prefix")
	q.Del("prefix")
	u.RawQuery = q.Encode()

	b, err := blob.OpenBucket(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("open blob bucket %q: %w", u.Scheme+"://"+u.Host, err)
	}
	if prefix != "" {
		b = blob.PrefixedBucket(b, strings.TrimLeft(prefix, "/"))
	}
	return &Bucket{b: b, signed: signedURLs}, nil
}

func (k *Bucket) Put(ctx context.Context, key string, r io.Reader) error {
	if err := validKey(key); err != nil {
		return err
	}
	w, err := k.b.NewWriter(ctx, key, &blob.WriterOptions{ContentType: contentTypeFor(key)})
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func (k *Bucket) Open(ctx context.Context, key string) (*Object, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	r, err := k.b.NewReader(ctx, key, nil)
	if gcerrors.Code(err) == gcerrors.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Object{ReadSeekCloser: r, Size: r.Size(), ModTime: r.ModTime()}, nil
}

func (k *Bucket) Delete(ctx context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	err := k.b.Delete(ctx, key)
	if gcerrors.Code(err) == gcerrors.NotFound {
		return nil
	}
	return err
}

func (k *Bucket) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if !k.signed {
		return "", nil
	}
	u, err := k.b.SignedURL(ctx, key, &blob.SignedURLOptions{Expiry: ttl, Method: "GET"})
	if gcerrors.Code(err) == gcerrors.Unimplemented {
		return "", nil
	}
	return u, err
}

func (k *Bucket) Close() error { return k.b.Close() }

func validKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid blob key %q", key)
	}
	return nil
}

// contentTypeFor sets sensible types so browsers and CDNs behave when they
// hit a signed URL directly.
func contentTypeFor(key string) string {
	if strings.HasSuffix(key, ".metadata") {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

var _ Store = (*Bucket)(nil)

// ErrUnsupportedScheme is returned by OpenURL for schemes without a driver.
var ErrUnsupportedScheme = errors.New("unsupported blob url scheme")
