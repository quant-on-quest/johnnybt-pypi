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

func TestLocalRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "blobs")
	l, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "abc123/johnnybt_demo-0.1.0-py3-none-any.whl"
	if err := l.Put(ctx, key, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	obj, err := l.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(obj)
	obj.Close()
	if string(b) != "hello" || obj.Size != 5 || obj.ModTime.IsZero() {
		t.Errorf("object = %q size=%d mod=%v", b, obj.Size, obj.ModTime)
	}
	// Replacing is atomic and leaves no temp files behind.
	if err := l.Put(ctx, key, strings.NewReader("hello again")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "abc123"))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
	if url, err := l.SignedURL(ctx, key, time.Minute); err != nil || url != "" {
		t.Errorf("local store has no signed URLs, got %q %v", url, err)
	}
	if err := l.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "abc123")); !errors.Is(err, os.ErrNotExist) {
		t.Error("empty per-file directory should be pruned")
	}
	if err := l.Delete(ctx, key); err != nil {
		t.Errorf("deleting a missing key should be a no-op, got %v", err)
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	l, _ := NewLocal(t.TempDir())
	for _, key := range []string{"../etc/passwd", "a/../../x", "", "/"} {
		if err := l.Put(ctx, key, strings.NewReader("x")); err == nil {
			t.Errorf("Put(%q) should be rejected", key)
		}
		if _, err := l.Open(ctx, key); err == nil {
			t.Errorf("Open(%q) should be rejected", key)
		}
	}
}
