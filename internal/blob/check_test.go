package blob

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckRoundTripsAProbeObject(t *testing.T) {
	s, _ := OpenURL(context.Background(), "mem://", true)
	defer s.Close()
	rep, err := Check(context.Background(), s, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Wrote || !rep.Read || !rep.Deleted || rep.SignedURL != "" || rep.SignedFetch {
		t.Errorf("report = %+v", rep)
	}
}

// signedStore wraps a store to hand out a URL served by a test server, so the
// fetch step of Check is exercised without a real S3.
type signedStore struct {
	Store
	url string
}

func (s signedStore) SignedURL(context.Context, string, time.Duration) (string, error) {
	return s.url, nil
}

func TestCheckFetchesTheSignedURL(t *testing.T) {
	inner, _ := OpenURL(context.Background(), "mem://", true)
	defer inner.Close()
	var served string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve whatever probe body was written, like a real bucket would.
		obj, err := inner.Open(r.Context(), served)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer obj.Close()
		io.Copy(w, obj)
	}))
	defer srv.Close()
	// Capture the probe key by observing writes.
	spy := &keySpy{Store: inner, onPut: func(k string) { served = k }}
	rep, err := Check(context.Background(), signedStore{Store: spy, url: srv.URL + "/probe"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SignedFetch || !strings.HasPrefix(rep.SignedURL, srv.URL) {
		t.Errorf("report = %+v", rep)
	}
}

type keySpy struct {
	Store
	onPut func(string)
}

func (k *keySpy) Put(ctx context.Context, key string, r io.Reader) error {
	k.onPut(key)
	return k.Store.Put(ctx, key, r)
}

func TestCheckReportsMismatch(t *testing.T) {
	inner, _ := OpenURL(context.Background(), "mem://", true)
	defer inner.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("garbage")) }))
	defer srv.Close()
	_, err := Check(context.Background(), signedStore{Store: inner, url: srv.URL}, srv.Client())
	if err == nil || !strings.Contains(err.Error(), "signed URL") {
		t.Errorf("expected a signed URL mismatch error, got %v", err)
	}
}

type noDelete struct{ Store }

func (noDelete) Delete(context.Context, string) error {
	return errors.New("AccessDenied: no oss:DeleteObject")
}

func TestCheckReportsDeleteFailure(t *testing.T) {
	inner, _ := OpenURL(context.Background(), "mem://", true)
	defer inner.Close()
	rep, err := Check(context.Background(), noDelete{inner}, http.DefaultClient)
	if err != nil {
		t.Fatalf("delete failure should not fail the whole check: %v", err)
	}
	if rep.Deleted || !strings.Contains(rep.DeleteError, "AccessDenied") {
		t.Errorf("report = %+v", rep)
	}
}
