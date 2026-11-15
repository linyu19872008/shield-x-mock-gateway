package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linyu19872008/shield-x-mock-gateway/agentlink"
)

// loadtestReport 是压测结束输出的 JSON 报告。
//
// 关键字段 achieved_rate：若明显低于 target_rate，必须能区分是
// 「mock 自身瓶颈」还是「shield-x 处理不过来」，否则会误导容量决策。
type loadtestReport struct {
	StartedAt     string           `json:"started_at"`
	DurationS     float64          `json:"duration_s"`
	TargetRate    int              `json:"target_rate"`
	AchievedRate  float64          `json:"achieved_rate"`
	Generated     int64            `json:"generated"`
	Enqueued      int64            `json:"enqueued"`
	Accepted      int64            `json:"accepted"`
	Rejected      int64            `json:"rejected"`
	DroppedClient int64            `json:"dropped_client"`
	BatchSent     int64            `json:"batch_sent"`
	HTTPErrors    int64            `json:"http_errors"`
	Retries       int64            `json:"retries"`
	SpoolPeak     int              `json:"spool_peak"`
	UploadLatency map[string]int64 `json:"upload_latency_ms"`
	Poll          pollReport       `json:"poll"`
	DryRun        bool             `json:"dry_run"`
	Notes         []string         `json:"notes"`
	FinalRevision int64            `json:"final_revision"`
	FinalPolicies int              `json:"final_policy_count"`
}

type pollReport struct {
	Count    int64   `json:"count"`
	P99MS    int64   `json:"p99_ms"`
	Ratio304 float64 `json:"ratio_304"`
	Failed   int64   `json:"failed"`
}

func cmdLoadTest(args []string) int {
	var f commonFlags
	var configPath, reportPath, rateArg string
	var duration time.Duration

	fs := flag.NewFlagSet("loadtest", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "YAML 配置文件路径")
	fs.StringVar(&f.baseURL, "base-url", "", "覆盖 shieldx.base_url（含 /shieldx 前缀）")
	fs.StringVar(&f.token, "token", "", "覆盖 shieldx.token")
	fs.StringVar(&f.gatewayID, "gateway-id", "", "覆盖 shieldx.gateway_id")
	fs.StringVar(&rateArg, "rate", "", "目标速率，支持 3000 / 3k")
	fs.DurationVar(&duration, "duration", 60*time.Second, "压测时长，如 10m")
	fs.StringVar(&reportPath, "report", "", "报告输出路径（默认打到 stdout）")
	fs.BoolVar(&f.dryRun, "dry-run", false, "只生成不上报，用于确认 mock 自身吞吐")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误：%v\n", err)
		return 1
	}
	f.apply(cfg)
	if rateArg != "" {
		r, err := parseRateArg(rateArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "非法 --rate：%v\n", err)
			return 1
		}
		cfg.Generate.Rate = r
	}
	if duration > 0 {
		cfg.Generate.Duration = Duration(duration)
	}
	if !f.dryRun && cfg.ShieldX.Token == "" {
		fmt.Fprintln(os.Stderr, "缺少 Token：请在配置或 --token 中提供（dry-run 可省略）")
		return 1
	}

	log := newLogger(cfg)
	st := &stats{}
	eng, err := newEngine(cfg, st, log, f.dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败：%v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	target := cfg.Generate.Rate
	effDuration := cfg.Generate.Duration.D()
	if effDuration <= 0 {
		effDuration = 60 * time.Second
	}
	deadline := time.Now().Add(effDuration)

	if f.dryRun {
		log("dry-run 模式：只生成不上报，用于确认 mock 自身能否达到 %d/s", target)
	} else {
		log("开始压测：rate=%d/s duration=%s base=%s", target, effDuration, cfg.ShieldX.BaseURL)
	}

	start := time.Now()
	eng.start(ctx, deadline)

	// 生成阶段：等到 deadline 或被中断
	select {
	case <-ctx.Done():
	case <-time.After(time.Until(deadline)):
	}

	// 生成已停：此处计时截止，避免把 flush 时间算进吞吐（会把达成率算低）
	genElapsed := time.Since(start).Seconds()

	// 给上报一点时间把积压发完
	flushWindow := 5 * time.Second
	if f.dryRun {
		flushWindow = 300 * time.Millisecond
	}
	log("生成阶段结束，等待 %s flush 积压 ...", flushWindow)
	select {
	case <-ctx.Done():
	case <-time.After(flushWindow):
	}

	stop() // 取消 ctx，让后台组件退出
	done := make(chan struct{})
	go func() { eng.wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log("等待后台组件退出超时")
	}

	totalElapsed := time.Since(start).Seconds()
	achieved := 0.0
	if genElapsed > 0 {
		achieved = float64(st.Generated.Load()) / genElapsed
	}

	ls := eng.linkStats()
	sn := eng.state.get()
	rep := loadtestReport{
		StartedAt:     start.Format(time.RFC3339),
		DurationS:     round2(totalElapsed),
		TargetRate:    target,
		AchievedRate:  round2(achieved),
		Generated:     st.Generated.Load(),
		Enqueued:      st.Enqueued.Load(),
		Accepted:      int64(ls.Accepted),
		Rejected:      int64(ls.Rejected),
		DroppedClient: int64(ls.Dropped),
		BatchSent:     int64(ls.Sent),
		HTTPErrors:    int64(ls.Retried),
		Retries:       int64(ls.Retried),
		SpoolPeak:     ls.SpoolEvents,
		UploadLatency: ls.UploadLatency,
		Poll: pollReport{
			Count:    int64(ls.PollSuccess),
			P99MS:    ls.PollLatency["p99"],
			Ratio304: ratio304(ls),
			Failed:   int64(ls.PollFailures),
		},
		DryRun:        f.dryRun,
		FinalRevision: sn.Revision,
		FinalPolicies: len(sn.Policies),
	}

	// 自检提示：区分「mock 瓶颈」与「shield-x 瓶颈」
	if target > 0 {
		ratio := achieved / float64(target)
		switch {
		case ratio < 0.95:
			rep.Notes = append(rep.Notes, fmt.Sprintf(
				"achieved_rate 仅为目标的 %.0f%%：先检查 mock 自身 CPU（用 --dry-run 复测），再判断是否为 shield-x 瓶颈",
				ratio*100))
		case ratio > 1.05:
			rep.Notes = append(rep.Notes, "achieved_rate 超过目标，令牌桶补发存在抖动，属正常")
		}
	}
	if ls.Dropped > 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"客户端丢弃 %d 条：上报队列已满，说明上报速度跟不上生成速度", ls.Dropped))
	}
	if rep.HTTPErrors > 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"%d 次请求失败，已落盘待补发或有数据缺口，请核对 spool 目录", rep.HTTPErrors))
	}

	out, _ := json.MarshalIndent(rep, "", "  ")
	if reportPath != "" {
		if err := os.WriteFile(reportPath, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "写报告失败：%v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "报告已写入 %s\n", reportPath)
		fmt.Println(string(out))
	} else {
		fmt.Println(string(out))
	}

	return 0
}

func ratio304(ls agentlink.Stats) float64 {
	total := ls.PollSuccess + ls.PollNotModified
	if total == 0 {
		return 0
	}
	return round2(float64(ls.PollNotModified) / float64(total))
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
