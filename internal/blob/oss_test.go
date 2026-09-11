package blob

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ossURL points the native OSS client at the in-process fake over plain HTTP.
func ossURL(srv *httptest.Server, extra string) string {
	host := strings.TrimPrefix(srv.URL, "http://")
	return "oss://test-bucket?region=cn-guangzhou&endpoint=" + host + "&disable_ssl=true&path_style=true" + extra
}

func TestOSSRoundTrip(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "LTAI-test")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "secret-test")
	fake, srv := newFakeS3(t) // the object REST surface is the same shape
	s, err := OpenOSS(context.Background(), ossURL(srv, ""), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	roundTrip(t, s)
	if len(fake.puts) == 0 {
		t.Fatal("no PUT recorded")
	}
	for _, h := range fake.puts {
		if h.Get("Content-Length") == "" {
			t.Error("PUT must carry Content-Length")
		}
		if h.Get("Authorization") == "" {
			t.Error("PUT must be signed")
		}
	}
}

func TestOSSPrefixSignedURLAndCheck(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "LTAI-test")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "secret-test")
	fake, srv := newFakeS3(t)
	s, err := OpenOSS(context.Background(), ossURL(srv, "&prefix=/pypi/"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Put(context.Background(), "k/x.whl", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.objects["test-bucket/pypi/k/x.whl"]; !ok {
		t.Errorf("prefix not applied; have %v", keysOf(fake.objects))
	}
	u, err := s.SignedURL(context.Background(), "k/x.whl", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/test-bucket/pypi/k/x.whl") || !strings.Contains(u, "x-oss-signature") {
		t.Errorf("signed url = %q", u)
	}
	rep, err := Check(context.Background(), s, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Wrote || !rep.Read || !rep.SignedFetch || !rep.Deleted {
		t.Errorf("report = %+v", rep)
	}

	off, err := OpenOSS(context.Background(), ossURL(srv, ""), false)
	if err != nil {
		t.Fatal(err)
	}
	defer off.Close()
	if u, err := off.SignedURL(context.Background(), "k", time.Minute); err != nil || u != "" {
		t.Errorf("disabled SignedURL = %q, %v", u, err)
	}
}

func TestOSSRangeReads(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "LTAI-test")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "secret-test")
	_, srv := newFakeS3(t)
	s, _ := OpenOSS(context.Background(), ossURL(srv, ""), true)
	defer s.Close()
	ctx := context.Background()
	if err := s.Put(ctx, "r.whl", strings.NewReader("0123456789")); err != nil {
		t.Fatal(err)
	}
	obj, err := s.Open(ctx, "r.whl")
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()
	// http.ServeContent's probing pattern: size via SeekEnd, then back, then a range.
	if n, _ := obj.Seek(0, 2); n != 10 {
		t.Errorf("SeekEnd = %d", n)
	}
	if n, _ := obj.Seek(3, 0); n != 3 {
		t.Errorf("SeekStart(3) = %d", n)
	}
	buf := make([]byte, 4)
	if _, err := obj.Read(buf); err != nil || string(buf) != "3456" {
		t.Errorf("read after seek = %q, %v", buf, err)
	}
	if n, _ := obj.Seek(2, 1); n != 9 {
		t.Errorf("SeekCurrent(+2) = %d", n)
	}
	rest := make([]byte, 8)
	n, _ := obj.Read(rest)
	if string(rest[:n]) != "9" {
		t.Errorf("tail = %q", rest[:n])
	}
}

func TestOSSURLValidation(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "x")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "y")
	cases := map[string]string{
		"oss://bucket":                        "region",
		"oss://?region=cn-guangzhou":          "bucket",
		"oss://bucket?region=cn-guangzhou&foo=1": "foo",
		"s3://bucket?region=cn-guangzhou":     "scheme",
	}
	for u, want := range cases {
		_, err := OpenOSS(context.Background(), u, true)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("OpenOSS(%q) err = %v, want mention of %q", u, err, want)
		}
	}
	s, err := OpenOSS(context.Background(), "oss://bucket?region=cn-guangzhou", true)
	if err != nil {
		t.Fatalf("minimal url: %v", err)
	}
	s.Close()
}

func TestOSSNeedsCredentials(t *testing.T) {
	t.Setenv("OSS_ACCESS_KEY_ID", "")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "")
	if _, err := OpenOSS(context.Background(), "oss://bucket?region=cn-guangzhou", true); err == nil || !strings.Contains(err.Error(), "OSS_ACCESS_KEY_ID") {
		t.Errorf("missing credentials should fail fast, got %v", err)
	}
}
