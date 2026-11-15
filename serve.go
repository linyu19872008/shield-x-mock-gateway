package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "YAML 配置文件路径")
	listen := fs.String("listen", "127.0.0.1:18080", "控制台监听地址")
	allowRemote := fs.Bool("allow-remote", false, "允许非 loopback 监听（必须同时提供 --ui-token）")
	uiToken := fs.String("ui-token", "", "控制台 Token；非 loopback 监听时必填")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if err := validateListen(*listen, *allowRemote); err != nil {
		fmt.Fprintf(os.Stderr, "监听地址非法：%v\n", err)
		return 1
	}
	if *allowRemote && *uiToken == "" {
		fmt.Fprintln(os.Stderr, "远程监听必须提供 --ui-token")
		return 1
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误：%v\n", err)
		return 1
	}

	ctrl := NewController(cfg, newLogger(cfg))
	handler, err := newServeHandler(ctrl, *uiToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 UI 失败：%v\n", err)
		return 1
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	fmt.Fprintf(os.Stderr, "mock-gateway 控制台：http://%s\n", *listen)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "控制台启动失败：%v\n", err)
			_ = ctrl.Stop()
			return 1
		}
	}

	_ = ctrl.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return 0
}

func validateListen(addr string, allowRemote bool) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if port == "" {
		return errors.New("缺少端口")
	}
	if allowRemote {
		return nil
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("默认只允许监听 loopback；远程访问请加 --allow-remote --ui-token")
}
