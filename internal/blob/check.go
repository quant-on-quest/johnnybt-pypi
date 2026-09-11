package blob

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CheckReport records which steps of a storage self-test succeeded.
type CheckReport struct {
	Key         string
	Wrote       bool
	Read        bool
	SignedURL   string
	SignedFetch bool
	Deleted     bool
}

// Check writes a probe object, reads it back, fetches it through a signed URL
// when the backend provides one, and deletes it. It is what
// `pypi-server blob check` runs so a new bucket can be verified before uploads.
func Check(ctx context.Context, s Store, client *http.Client) (*CheckReport, error) {
	var nonce [8]byte
	rand.Read(nonce[:])
	rep := &CheckReport{Key: "_probe/" + hex.EncodeToString(nonce[:]) + ".txt"}
	body := []byte("johnnybt-pypi storage probe " + rep.Key + "\n")

	if err := s.Put(ctx, rep.Key, bytes.NewReader(body)); err != nil {
		return rep, fmt.Errorf("write: %w", err)
	}
	rep.Wrote = true
	defer func() {
		if err := s.Delete(ctx, rep.Key); err == nil {
			rep.Deleted = true
		}
	}()

	obj, err := s.Open(ctx, rep.Key)
	if err != nil {
		return rep, fmt.Errorf("read back: %w", err)
	}
	got, err := io.ReadAll(obj)
	obj.Close()
	if err != nil || !bytes.Equal(got, body) {
		return rep, fmt.Errorf("read back: content mismatch (%v)", err)
	}
	rep.Read = true

	u, err := s.SignedURL(ctx, rep.Key, 2*time.Minute)
	if err != nil {
		return rep, fmt.Errorf("sign url: %w", err)
	}
	if u == "" {
		return rep, nil // backend streams through the server; nothing more to test
	}
	rep.SignedURL = u
	res, err := client.Get(u)
	if err != nil {
		return rep, fmt.Errorf("fetch signed URL: %w", err)
	}
	defer res.Body.Close()
	fetched, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.Equal(fetched, body) {
		return rep, fmt.Errorf("fetch signed URL: status %d, body %q", res.StatusCode, truncate(fetched, 200))
	}
	rep.SignedFetch = true
	return rep, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
