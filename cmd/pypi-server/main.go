// Command pypi-server runs the private package index.
//
//	pypi-server                      start the server
//	pypi-server admin reset-password print a fresh admin password
//	pypi-server blob check           verify the configured object store
//	pypi-server version              print the build version
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/config"
	"github.com/quant-on-quest/johnnybt-pypi/internal/server"
)

// version is stamped by the release build: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	args := os.Args[1:]
	switch {
	case len(args) == 0 || args[0] == "serve":
		os.Exit(serve(cfg, log))
	case len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v"):
		fmt.Println(version)
	case len(args) == 2 && args[0] == "blob" && args[1] == "check":
		os.Exit(blobCheck(cfg))
	case len(args) == 2 && args[0] == "admin" && args[1] == "reset-password":
		user, pw, err := server.ResetAdminPassword(context.Background(), cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("管理员密码已重置\n  用户名: %s\n  密码:   %s\n", user, pw)
	default:
		fmt.Fprintln(os.Stderr, "usage: pypi-server [serve | admin reset-password | blob check | version]")
		os.Exit(2)
	}
}

// blobCheck exercises the object store end to end and prints what worked.
func blobCheck(cfg config.Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target := cfg.BlobURL
	if target == "" {
		target = "local:" + cfg.BlobDir()
	}
	fmt.Printf("存储: %s\n签名 URL: %v\n", target, cfg.BlobSignedURLs)
	store, err := server.OpenBlobs(ctx, cfg)
	if err != nil {
		fmt.Println("✗ 打开失败:", err)
		return 1
	}
	defer store.Close()
	rep, err := blob.Check(ctx, store, &http.Client{Timeout: 30 * time.Second})
	mark := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}
	fmt.Printf("%s 写入探测对象 %s\n", mark(rep.Wrote), rep.Key)
	fmt.Printf("%s 读回并比对\n", mark(rep.Read))
	if rep.SignedURL != "" {
		fmt.Printf("%s 通过签名 URL 下载（%s）\n", mark(rep.SignedFetch), rep.SignedURL)
	} else {
		fmt.Println("- 该后端不提供签名 URL，下载将由服务器转发")
	}
	fmt.Printf("%s 删除探测对象\n", mark(rep.Deleted))
	if err != nil {
		fmt.Println("✗", err)
		return 1
	}
	fmt.Println("存储配置可用")
	return 0
}

func serve(cfg config.Config, log *slog.Logger) int {
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Error("startup failed", "err", err)
		return 1
	}
	defer srv.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	user, pw, created, err := srv.EnsureAdmin(ctx)
	if err != nil {
		log.Error("bootstrap admin", "err", err)
		return 1
	}
	if created {
		path, werr := server.WriteInitialPasswordFile(cfg, user, pw)
		fmt.Printf("\n"+
			"==========================================================\n"+
			"  首次启动，已创建管理员账号（密码只显示这一次）\n"+
			"    用户名: %s\n"+
			"    密码:   %s\n", user, pw)
		if werr == nil {
			fmt.Printf("  已同时写入 %s，登录后请删除\n", path)
		} else {
			fmt.Printf("  （写入密码文件失败: %v）\n", werr)
		}
		fmt.Printf("  忘记密码可执行: pypi-server admin reset-password\n" +
			"==========================================================\n\n")
	}

	log.Info("starting", "version", version, "data_dir", cfg.DataDir, "admin_path", cfg.AdminPath)
	if err := srv.Run(ctx); err != nil {
		log.Error("server stopped", "err", err)
		return 1
	}
	log.Info("bye")
	return 0
}
