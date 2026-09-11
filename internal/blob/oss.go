package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

// OSS stores blobs in Alibaba Cloud OSS through the official SDK. The S3
// compatibility layer rejects the AWS SDK's chunked/trailing-checksum uploads,
// so OSS gets a native backend; other S3-compatible stores use Bucket.
//
// URL: oss://<bucket>?region=cn-guangzhou[&endpoint=host][&internal=true][&prefix=dir/]
// Credentials come from OSS_ACCESS_KEY_ID / OSS_ACCESS_KEY_SECRET.
type OSS struct {
	client *oss.Client
	bucket string
	prefix string
	signed bool
}

func OpenOSS(ctx context.Context, rawURL string, signedURLs bool) (*OSS, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("oss url: %w", err)
	}
	if u.Scheme != "oss" {
		return nil, fmt.Errorf("oss url: unexpected scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("oss url: missing bucket (oss://<bucket>?region=…)")
	}
	q := u.Query()
	region := q.Get("region")
	if region == "" {
		return nil, errors.New("oss url: region is required, e.g. region=cn-guangzhou")
	}
	if os.Getenv("OSS_ACCESS_KEY_ID") == "" || os.Getenv("OSS_ACCESS_KEY_SECRET") == "" {
		return nil, errors.New("oss: OSS_ACCESS_KEY_ID and OSS_ACCESS_KEY_SECRET must be set")
	}
	for k := range q {
		switch k {
		case "region", "endpoint", "internal", "prefix", "disable_ssl", "path_style":
		default:
			return nil, fmt.Errorf("oss url: unknown parameter %q", k)
		}
	}

	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewEnvironmentVariableCredentialsProvider()).
		WithRegion(strings.TrimPrefix(region, "oss-")).
		WithSignatureVersion(oss.SignatureVersionV4)
	if ep := q.Get("endpoint"); ep != "" {
		cfg = cfg.WithEndpoint(ep)
	}
	if q.Get("internal") == "true" {
		cfg = cfg.WithUseInternalEndpoint(true)
	}
	if q.Get("disable_ssl") == "true" {
		cfg = cfg.WithDisableSSL(true)
	}
	if q.Get("path_style") == "true" {
		cfg = cfg.WithUsePathStyle(true)
	}
	return &OSS{
		client: oss.NewClient(cfg),
		bucket: u.Host,
		prefix: strings.TrimLeft(q.Get("prefix"), "/"),
		signed: signedURLs,
	}, nil
}

func (o *OSS) key(k string) *string { return oss.Ptr(o.prefix + k) }

func (o *OSS) Put(ctx context.Context, key string, r io.Reader) error {
	if err := validKey(key); err != nil {
		return err
	}
	req := &oss.PutObjectRequest{
		Bucket:      oss.Ptr(o.bucket),
		Key:         o.key(key),
		Body:        r,
		ContentType: oss.Ptr(contentTypeFor(key)),
	}
	// A known length lets the SDK send a plain body with Content-Length.
	if size, ok := readerSize(r); ok {
		req.ContentLength = oss.Ptr(size)
	}
	_, err := o.client.PutObject(ctx, req)
	return err
}

func (o *OSS) Open(ctx context.Context, key string) (*Object, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	head, err := o.client.HeadObject(ctx, &oss.HeadObjectRequest{Bucket: oss.Ptr(o.bucket), Key: o.key(key)})
	if err != nil {
		return nil, ossErr(err)
	}
	mod := time.Now()
	if head.LastModified != nil {
		mod = *head.LastModified
	}
	rr := &rangeReader{
		size: head.ContentLength,
		openAt: func(off int64) (io.ReadCloser, error) {
			res, err := o.client.GetObject(ctx, &oss.GetObjectRequest{
				Bucket:        oss.Ptr(o.bucket),
				Key:           o.key(key),
				Range:         oss.Ptr("bytes=" + strconv.FormatInt(off, 10) + "-"),
				RangeBehavior: oss.Ptr("standard"),
			})
			if err != nil {
				return nil, ossErr(err)
			}
			return res.Body, nil
		},
	}
	return &Object{ReadSeekCloser: rr, Size: head.ContentLength, ModTime: mod}, nil
}

func (o *OSS) Delete(ctx context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	_, err := o.client.DeleteObject(ctx, &oss.DeleteObjectRequest{Bucket: oss.Ptr(o.bucket), Key: o.key(key)})
	if errors.Is(ossErr(err), ErrNotFound) {
		return nil
	}
	return err
}

func (o *OSS) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if !o.signed {
		return "", nil
	}
	res, err := o.client.Presign(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(o.bucket), Key: o.key(key)},
		func(po *oss.PresignOptions) { po.Expires = ttl })
	if err != nil {
		return "", err
	}
	return res.URL, nil
}

func (o *OSS) Close() error { return nil }

var _ Store = (*OSS)(nil)

// ossErr maps the SDK's 404s to ErrNotFound.
func ossErr(err error) error {
	if err == nil {
		return nil
	}
	var se *oss.ServiceError
	if errors.As(err, &se) && (se.StatusCode == 404 || se.Code == "NoSuchKey") {
		return ErrNotFound
	}
	return err
}

// readerSize reports the remaining length of a seekable reader without
// consuming it.
func readerSize(r io.Reader) (int64, bool) {
	s, ok := r.(io.Seeker)
	if !ok {
		return 0, false
	}
	cur, err := s.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, false
	}
	end, err := s.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, false
	}
	if _, err := s.Seek(cur, io.SeekStart); err != nil {
		return 0, false
	}
	return end - cur, true
}

// rangeReader turns "GET with Range" into an io.ReadSeekCloser, which is what
// http.ServeContent needs. The body is opened lazily at the current offset and
// reopened after every Seek.
type rangeReader struct {
	size   int64
	off    int64
	openAt func(off int64) (io.ReadCloser, error)
	body   io.ReadCloser
}

func (r *rangeReader) Read(p []byte) (int, error) {
	if r.off >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		b, err := r.openAt(r.off)
		if err != nil {
			return 0, err
		}
		r.body = b
	}
	n, err := r.body.Read(p)
	r.off += int64(n)
	return n, err
}

func (r *rangeReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.off + offset
	case io.SeekEnd:
		abs = r.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative position")
	}
	if abs != r.off && r.body != nil {
		r.body.Close()
		r.body = nil
	}
	r.off = abs
	return abs, nil
}

func (r *rangeReader) Close() error {
	if r.body != nil {
		err := r.body.Close()
		r.body = nil
		return err
	}
	return nil
}
