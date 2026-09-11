// Package server wires the HTTP mux, middleware and TLS.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/quant-on-quest/johnnybt-pypi/internal/api"
	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/config"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pypi"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
	"github.com/quant-on-quest/johnnybt-pypi/internal/web"
)

type Server struct {
	cfg   config.Config
	log   *slog.Logger
	store *store.Store
	blobs blob.Store
	mux   *http.ServeMux
}

func New(cfg config.Config, log *slog.Logger) (*Server, error) {
	for _, dir := range []string{cfg.DataDir, cfg.BlobDir(), cfg.TmpDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	blobs, err := OpenBlobs(context.Background(), cfg)
	if err != nil {
		st.Close()
		return nil, err
	}
	s := &Server{cfg: cfg, log: log, store: st, blobs: blobs, mux: http.NewServeMux()}

	ingester := &pypi.Ingester{Store: st, Blobs: blobs, TmpDir: cfg.TmpDir(), MaxSize: cfg.MaxUploadSize}
	(&pypi.Handler{Store: st, Blobs: blobs, Ingester: ingester, BaseURL: cfg.BaseURL, Log: log}).Register(s.mux)

	apiMux := http.NewServeMux()
	(&api.API{Store: st, Blobs: blobs, Ingester: ingester, Cfg: cfg, Log: log}).Register(apiMux)
	// Browser-originated cross-site requests are rejected via Sec-Fetch-Site /
	// Origin; pip and twine never hit /api so they are unaffected.
	s.mux.Handle("/api/", http.NewCrossOriginProtection().Handler(apiMux))

	s.mux.Handle("/", web.Handler(web.Options{AdminPath: cfg.AdminPath}))
	return s, nil
}

// OpenBlobs picks the object store from the configuration: the local blobs/
// directory by default, otherwise whatever PYPI_BLOB_URL points at.
func OpenBlobs(ctx context.Context, cfg config.Config) (blob.Store, error) {
	if cfg.BlobURL == "" {
		return blob.NewLocal(cfg.BlobDir())
	}
	if strings.HasPrefix(cfg.BlobURL, "oss://") {
		b, err := blob.OpenOSS(ctx, cfg.BlobURL, cfg.BlobSignedURLs)
		if err != nil {
			return nil, fmt.Errorf("blob store: %w", err)
		}
		return b, nil
	}
	b, err := blob.OpenURL(ctx, cfg.BlobURL, cfg.BlobSignedURLs)
	if err != nil {
		return nil, fmt.Errorf("blob store: %w", err)
	}
	return b, nil
}

func (s *Server) Store() *store.Store { return s.store }

func (s *Server) Close() error {
	s.blobs.Close()
	return s.store.Close()
}

// Handler is the fully wrapped root handler.
func (s *Server) Handler() http.Handler {
	return s.recoverer(s.requestLogger(s.mux))
}

// EnsureAdmin creates the initial admin account on first start and reports
// its password so it can be printed exactly once.
func (s *Server) EnsureAdmin(ctx context.Context) (username, password string, created bool, err error) {
	if _, err := s.store.AdminUser(ctx); err == nil {
		return "", "", false, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", "", false, err
	}
	password = auth.GeneratePassword()
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", "", false, err
	}
	if _, err := s.store.CreateUser(ctx, "admin", "初始管理员", true, hash); err != nil {
		return "", "", false, err
	}
	return "admin", password, true, nil
}

// ResetAdminPassword generates a new password for the first admin account.
func ResetAdminPassword(ctx context.Context, cfg config.Config) (username, password string, err error) {
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return "", "", err
	}
	defer st.Close()
	u, err := st.AdminUser(ctx)
	if err != nil {
		return "", "", fmt.Errorf("no admin account yet; start the server once first")
	}
	password = auth.GeneratePassword()
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", "", err
	}
	if err := st.SetPassword(ctx, u.ID, hash); err != nil {
		return "", "", err
	}
	_ = st.DeleteUserSessions(ctx, u.ID)
	return u.Username, password, nil
}

// Run serves until ctx is cancelled. With TLS domains configured it binds
// :443 for HTTPS and :80 for ACME challenges + redirects; otherwise it serves
// plain HTTP on cfg.Addr.
func (s *Server) Run(ctx context.Context) error {
	handler := s.Handler()
	if len(s.cfg.TLSDomains) == 0 {
		srv := &http.Server{Addr: s.cfg.Addr, Handler: handler, ReadHeaderTimeout: 15 * time.Second}
		s.log.Info("listening", "addr", s.cfg.Addr, "tls", false)
		return serveUntil(ctx, srv, func() error { return srv.ListenAndServe() })
	}

	if err := os.MkdirAll(s.cfg.CertDir(), 0o700); err != nil {
		return err
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(s.cfg.CertDir()),
		HostPolicy: autocert.HostWhitelist(s.cfg.TLSDomains...),
		Email:      s.cfg.TLSEmail,
	}
	httpSrv := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           m.HTTPHandler(http.HandlerFunc(redirectHTTPS)),
		ReadHeaderTimeout: 15 * time.Second,
	}
	tlsSrv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           handler,
		TLSConfig:         m.TLSConfig(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	tlsSrv.TLSConfig.MinVersion = tls.VersionTLS12

	errCh := make(chan error, 2)
	go func() {
		s.log.Info("listening", "addr", httpSrv.Addr, "role", "acme+redirect")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http listener: %w", err)
		}
	}()
	go func() {
		s.log.Info("listening", "addr", tlsSrv.Addr, "tls", true, "domains", strings.Join(s.cfg.TLSDomains, ","))
		if err := tlsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("https listener: %w", err)
		}
	}()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	return tlsSrv.Shutdown(shutdownCtx)
}

func serveUntil(ctx context.Context, srv *http.Server, listen func() error) error {
	errCh := make(chan error, 1)
	go func() {
		if err := listen(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func redirectHTTPS(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	target := "https://" + host + r.URL.RequestURI()
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// --- middleware -------------------------------------------------------------

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	n, err := sw.ResponseWriter.Write(b)
	sw.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		// Static UI assets are noise; everything else is worth a line.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"dur", time.Since(start).Round(time.Millisecond).String(),
			"ip", clientIP(r),
			"ua", truncate(r.UserAgent(), 80),
		)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "path", r.URL.Path, "err", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// WriteInitialPasswordFile stores the bootstrap password next to the data so
// it survives a scrolled-away terminal.
func WriteInitialPasswordFile(cfg config.Config, username, password string) (string, error) {
	p := cfg.AdminPWFile()
	content := fmt.Sprintf("username: %s\npassword: %s\n\nDelete this file once you have logged in.\n", username, password)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p, nil
	}
	return abs, nil
}
