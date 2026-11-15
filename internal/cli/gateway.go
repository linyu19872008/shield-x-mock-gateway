package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/config"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/dataplane"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/engine"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/logging"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/state"
)

// Gateway runs the HTTP data plane.
func Gateway(args []string) int {
	fs := flag.NewFlagSet("gateway", flag.ContinueOnError)
	configPath := fs.String("config", "", "YAML 配置文件路径")
	listen := fs.String("listen", "", "数据面监听地址（默认取 ingress.listen）")
	allowRemote := fs.Bool("allow-remote", false, "允许非 loopback 监听")
	baseURL := fs.String("base-url", "", "覆盖 shieldx.base_url")
	token := fs.String("token", "", "覆盖 shieldx.token")
	gatewayID := fs.String("gateway-id", "", "覆盖 shieldx.gateway_id")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误：%v\n", err)
		return 1
	}
	if *baseURL != "" {
		cfg.ShieldX.BaseURL = trimTrailingSlash(*baseURL)
	}
	if *token != "" {
		cfg.ShieldX.Token = *token
	}
	if *gatewayID != "" {
		cfg.ShieldX.GatewayID = *gatewayID
	}
	if *listen != "" {
		cfg.Ingress.Listen = *listen
	}
	if *allowRemote {
		cfg.Ingress.AllowRemote = true
	}
	if cfg.ShieldX.Token == "" {
		fmt.Fprintln(os.Stderr, "缺少 Token：请在配置或 --token 中提供租户 Token")
		return 1
	}
	if err := validateListen(cfg.Ingress.Listen, cfg.Ingress.AllowRemote); err != nil {
		fmt.Fprintf(os.Stderr, "监听地址非法：%v\n", err)
		return 1
	}

	log := logging.New()
	eng, err := engine.New(cfg, &state.Stats{}, log, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败：%v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := eng.StartAgent(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "启动 agentlink 失败：%v\n", err)
		return 1
	}

	handler := dataplane.NewHandler(cfg, eng.State(), eng.Templates(), eng.Enqueue, log)
	srv := &http.Server{
		Addr:              cfg.Ingress.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.Ingress.ReadTimeout.D(),
		WriteTimeout:      cfg.Ingress.WriteTimeout.D(),
		IdleTimeout:       30 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log("mock-gateway 数据面：http://%s（不代理业务系统）", cfg.Ingress.Listen)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log("数据面启动失败：%v", err)
			stop()
			eng.Wait()
			return 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	stop()
	eng.Wait()
	return 0
}
