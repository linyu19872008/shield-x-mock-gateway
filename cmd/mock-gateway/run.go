package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// commonFlags 是 run / loadtest 共用的覆盖参数。
type commonFlags struct {
	config    string
	baseURL   string
	token     string
	gatewayID string
	rate      int
	duration  time.Duration
	dryRun    bool
}

func (c *commonFlags) apply(cfg *Config) {
	if c.baseURL != "" {
		cfg.ShieldX.BaseURL = trimTrailingSlash(c.baseURL)
	}
	if c.token != "" {
		cfg.ShieldX.Token = c.token
	}
	if c.gatewayID != "" {
		cfg.ShieldX.GatewayID = c.gatewayID
	}
	if c.rate > 0 {
		cfg.Generate.Rate = c.rate
	}
	if c.duration > 0 {
		cfg.Generate.Duration = Duration(c.duration)
	}
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func cmdRun(args []string) int {
	var f commonFlags
	var configPath string
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "YAML 配置文件路径")
	fs.StringVar(&f.baseURL, "base-url", "", "覆盖 shieldx.base_url（含 /shieldx 前缀）")
	fs.StringVar(&f.token, "token", "", "覆盖 shieldx.token")
	fs.StringVar(&f.gatewayID, "gateway-id", "", "覆盖 shieldx.gateway_id")
	fs.IntVar(&f.rate, "rate", 0, "覆盖 generate.rate（0 表示用配置）")
	backfill := fs.Duration("backfill", 0, "回填模式：把 rate×该时长 条事件的时间戳铺满这段时间（如 1h），并尽快发完")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误：%v\n", err)
		return 1
	}
	f.apply(cfg)
	if *backfill > 0 {
		cfg.Generate.Backfill = Duration(*backfill)
	}
	if cfg.Generate.Backfill.D() > 0 {
		if cfg.Generate.Rate <= 0 {
			fmt.Fprintln(os.Stderr, "回填模式需要 rate > 0（每条事件代表的时间密度）")
			return 1
		}
		if cfg.Generate.Rate > 5000 {
			fmt.Fprintln(os.Stderr, "回填模式 rate 建议 <= 5000，否则单次生成量过大")
			return 1
		}
	}

	if cfg.ShieldX.Token == "" {
		fmt.Fprintln(os.Stderr, "缺少 Token：请在配置或 --token 中提供租户 Token")
		return 1
	}

	log := newLogger(cfg)
	st := &stats{}
	eng, err := newEngine(cfg, st, log, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败：%v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if d := cfg.Generate.Backfill.D(); d > 0 {
		total := int64(cfg.Generate.Rate) * int64(d.Seconds())
		log("启动 mock-gateway（回填模式）：base=%s gateway_id=%s 时间窗=%s rate=%d/s 共 %d 条 workers=%d format=%s",
			cfg.ShieldX.BaseURL, cfg.ShieldX.GatewayID, d, cfg.Generate.Rate, total,
			cfg.Generate.Workers, cfg.Report.Format)
	} else {
		log("启动 mock-gateway：base=%s gateway_id=%s rate=%d/s workers=%d format=%s",
			cfg.ShieldX.BaseURL, cfg.ShieldX.GatewayID, cfg.Generate.Rate,
			cfg.Generate.Workers, cfg.Report.Format)
	}

	eng.start(ctx, time.Time{})

	// 回填模式：等生成+上报完毕后主动退出，不需要用户 Ctrl-C
	if cfg.Generate.Backfill.D() > 0 {
		eng.gen.wait()
		// 等上报队列排空（最多 60s），避免最后几批还在途就退出
		drainDeadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(drainDeadline) && eng.queueDepth() > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond)
		printSummary(st, eng)
		stop()
		done := make(chan struct{})
		go func() { eng.wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			log("等待后台组件退出超时")
		}
		return 0
	}

	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				printStatus(st, eng)
			}
		}
	}()

	<-ctx.Done()
	log("收到退出信号，正在 flush ...")

	done := make(chan struct{})
	go func() { eng.wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		log("等待后台组件退出超时")
	}

	printSummary(st, eng)
	return 0
}

func printStatus(st *stats, eng *engine) {
	ls := eng.linkStats()
	sn := eng.state.get()
	fmt.Fprintf(os.Stderr,
		"[run] gen=%d queue=%d sent=%d accepted=%d rejected=%d dropped=%d spool=%d | policy_rev=%d policies=%d templates=%d | poll ok=%d 304=%d fail=%d\n",
		st.Generated.Load(), ls.QueueDepth, ls.Sent,
		ls.Accepted, ls.Rejected, ls.Dropped, ls.SpoolEvents,
		sn.Revision, len(sn.Policies), eng.cache.len(),
		ls.PollSuccess, ls.PollNotModified, ls.PollFailures)
}

func printSummary(st *stats, eng *engine) {
	ls := eng.linkStats()
	fmt.Fprintf(os.Stderr,
		"\n汇总：生成 %d，入队 %d，已发批次 %d，服务端接收 %d，拒绝 %d，客户端丢弃 %d，落盘补发 %d\n",
		st.Generated.Load(), st.Enqueued.Load(), ls.Sent,
		ls.Accepted, ls.Rejected, ls.Dropped, ls.Replayed)
}

func newLogger(cfg *Config) logger {
	return func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "[%s] %s\n",
			time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	}
}

// parseRateArg 支持 "3000" 与 "3k" 两种写法。
func parseRateArg(s string) (int, error) {
	if len(s) > 1 && (s[len(s)-1] == 'k' || s[len(s)-1] == 'K') {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return n * 1000, nil
	}
	return strconv.Atoi(s)
}
