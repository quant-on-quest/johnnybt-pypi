package blob

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func roundTrip(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	key := "abc123/johnnybt_demo-0.1.0-py3-none-any.whl"
	if err := s.Put(ctx, key, strings.NewReader("hello wheel")); err != nil {
		t.Fatal("put:", err)
	}
	obj, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal("open:", err)
	}
	if obj.Size != 11 || obj.ModTime.IsZero() {
		t.Errorf("size=%d mod=%v", obj.Size, obj.ModTime)
	}
	// Seeking is what http.ServeContent needs for range requests.
	if _, err := obj.Seek(6, io.SeekStart); err != nil {
		t.Fatal("seek:", err)
	}
	rest, _ := io.ReadAll(obj)
	obj.Close()
	if string(rest) != "wheel" {
		t.Errorf("after seek: %q", rest)
	}
	if err := s.Put(ctx, key, strings.NewReader("replaced")); err != nil {
		t.Fatal("overwrite:", err)
	}
	obj, _ = s.Open(ctx, key)
	b, _ := io.ReadAll(obj)
	obj.Close()
	if string(b) != "replaced" {
		t.Errorf("overwrite not visible: %q", b)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal("delete:", err)
	}
	if _, err := s.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("open after delete: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Errorf("deleting a missing key should be a no-op: %v", err)
	}
	if _, err := s.Open(ctx, "../etc/passwd"); err == nil {
		t.Error("traversal key accepted")
	}
}

func TestMemBucket(t *testing.T) {
	s, err := OpenURL(context.Background(), "mem://", true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	roundTrip(t, s)
	// mem has no signer: fall back to streaming, not an error.
	if url, err := s.SignedURL(context.Background(), "k", time.Minute); err != nil || url != "" {
		t.Errorf("SignedURL = %q, %v", url, err)
	}
}

func TestFileBucket(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenURL(context.Background(), "file://"+dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	roundTrip(t, s)
	if err := s.Put(context.Background(), "x/y.whl", strings.NewReader("z")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x", "y.whl")); err != nil {
		t.Errorf("file bucket should write under the directory: %v", err)
	}
}

func TestPrefixParam(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenURL(context.Background(), "file://"+dir+"?prefix=pypi/blobs/", true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Put(context.Background(), "k.whl", strings.NewReader("z")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pypi", "blobs", "k.whl")); err != nil {
		t.Errorf("prefix not applied: %v", err)
	}
}

func TestS3SignedURLIsLocalAndOptional(t *testing.T) {
	// Presigning never touches the network, so an unreachable endpoint is fine.
	t.Setenv("AWS_ACCESS_KEY_ID", "test-ak")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-sk")
	url := "s3://my-bucket?endpoint=http://127.0.0.1:9&region=oss-cn-hangzhou&use_path_style=true&disable_https=true"
	s, err := OpenURL(context.Background(), url, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	signed, err := s.SignedURL(context.Background(), "sha/johnnybt_demo-0.1.0-py3-none-any.whl", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"127.0.0.1:9", "my-bucket", "johnnybt_demo-0.1.0-py3-none-any.whl", "X-Amz-Signature=", "X-Amz-Expires=600"} {
		if !strings.Contains(signed, want) {
			t.Errorf("signed url missing %q: %s", want, signed)
		}
	}
	// Signed URLs can be switched off to force streaming through the server.
	s2, err := OpenURL(context.Background(), url, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if u, err := s2.SignedURL(context.Background(), "k", time.Minute); err != nil || u != "" {
		t.Errorf("disabled SignedURL = %q, %v", u, err)
	}
}

func TestOpenURLRejectsUnknownScheme(t *testing.T) {
	if _, err := OpenURL(context.Background(), "ftp://x", true); err == nil {
		t.Error("unknown scheme accepted")
	}
}

func TestContentTypeForKey(t *testing.T) {
	cases := map[string]string{
		"a/x.whl":          "application/octet-stream",
		"a/x.tar.gz":       "application/octet-stream",
		"a/x.whl.metadata": "text/plain; charset=utf-8",
	}
	for k, want := range cases {
		if got := contentTypeFor(k); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", k, got, want)
		}
	}
}
