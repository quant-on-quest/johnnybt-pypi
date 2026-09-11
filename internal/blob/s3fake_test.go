package blob

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is the smallest object store our code paths need (path-style PUT/GET/
// HEAD/DELETE, ranges, S3-style 404 bodies). Both the s3:// and oss:// backends
// are exercised against it. Like Aliyun OSS's S3 layer it rejects aws-chunked /
// trailing-checksum uploads.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	puts    []http.Header
}

func newFakeS3(t *testing.T) (*fakeS3, *httptest.Server) {
	t.Helper()
	f := &fakeS3{objects: map[string][]byte{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		f.puts = append(f.puts, r.Header.Clone())
		sha := r.Header.Get("X-Amz-Content-Sha256")
		if strings.HasPrefix(sha, "STREAMING-") || strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `<Error><Code>NotImplemented</Code><Message>Aws MultiChunkedEncoding %s is not supported.</Message></Error>`, sha)
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.objects[key] = b
		w.Header().Set("ETag", `"fake"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet, http.MethodHead:
		b, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		start, end := 0, len(b)-1
		partial := false
		if rg := r.Header.Get("Range"); strings.HasPrefix(rg, "bytes=") {
			partial = true
			parts := strings.SplitN(strings.TrimPrefix(rg, "bytes="), "-", 2)
			start, _ = strconv.Atoi(parts[0])
			if parts[1] != "" {
				end, _ = strconv.Atoi(parts[1])
			}
			if end >= len(b) {
				end = len(b) - 1
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("ETag", `"fake"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Accept-Ranges", "bytes")
		if partial {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(b)))
			w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			w.WriteHeader(http.StatusOK)
		}
		if r.Method == http.MethodGet {
			w.Write(b[start : end+1])
		}
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func s3URL(srv *httptest.Server, extra string) string {
	return "s3://test-bucket?endpoint=" + srv.URL + "&region=oss-cn-guangzhou&use_path_style=true&disable_https=true" +
		"&request_checksum_calculation=when_required&response_checksum_validation=when_required" + extra
}
