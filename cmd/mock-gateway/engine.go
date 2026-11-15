package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/linyu19872008/shield-x-client-go/agent"
	"github.com/linyu19872008/shield-x-mock-gateway/cmd/mock-gateway/agentlink"
)

// engine 把 agentlink（SDK 轮询/上报）与 generator 串成一条流水线。
//
// 求值、模板渲染、流量生成仍由 mock 独立实现；所有 agent 调用走 agentlink。
type engine struct {
	cfg    *Config
	st     *stats
	log    logger
	dryRun bool

	ft    *faultTransport
	state *policyState
	cache *templateCache
	link  *agentlink.Link
	gen   *generator

	wg sync.WaitGroup
}

func newEngine(cfg *Config, st *stats, log logger, dryRun bool) (*engine, error) {
	state := &policyState{}
	cache := newTemplateCache()
	ft := newFaultTransport(cfg.Fault)

	gen := &generator{
		cfg:   cfg,
		state: state,
		tpl:   cache,
		st:    st,
	}
	gen.initBackfill()

	e := &engine{
		cfg:    cfg,
		st:     st,
		log:    log,
		dryRun: dryRun,
		ft:     ft,
		state:  state,
		cache:  cache,
		gen:    gen,
	}
	if dryRun {
		return e, nil
	}

	initial := loadSnapshot(cfg, state, log)

	var link *agentlink.Link
	link, err := agentlink.New(agentlink.Options{
		BaseURL:         cfg.ShieldX.BaseURL,
		Token:           cfg.ShieldX.Token,
		GatewayID:       cfg.ShieldX.GatewayID,
		AgentVersion:    cfg.ShieldX.AgentVersion,
		Capabilities:    toAgentCapabilities(cfg.ShieldX.Capabilities),
		PollInterval:    cfg.Poll.Interval.D(),
		PollTimeout:     cfg.Poll.Timeout.D(),
		InitialRevision: initial,
		OnPolicies: func(s *agent.PolicySnapshot) {
			snap := toSnapshot(s, cfg.ShieldX.BaseURL)
			state.apply(snap)
			saveSnapshot(cfg, snap, log)
			if cfg.Template.FetchOnMissing && len(missingTemplates(snap.Policies, cache)) > 0 {
				go func() {
					if err := link.RefreshTemplates(context.Background()); err != nil {
						log("警告：拉模板失败：%v（将使用内置兜底页）", err)
					}
				}()
			}
		},
		OnTemplates: func(s *agent.TemplateSnapshot) {
			cache.replace(toTemplates(s))
			log("模板已更新：revision=%d templates=%d", s.Revision, len(s.Templates))
		},
		OnError: func(err error) {
			log("agent 错误：%v", err)
		},
		ReportInterval: cfg.Report.Interval.D(),
		BatchSize:      cfg.Report.BatchSize,
		Format:         agent.Format(cfg.Report.Format),
		DisableGzip:    !cfg.Report.Gzip,
		ReportTimeout:  cfg.Report.Timeout.D(),
		MaxBackoff:     cfg.Report.MaxBackoff.D(),
		SpoolDir:       cfg.Report.SpoolDir,
		SpoolMaxMB:     cfg.Report.SpoolMaxMB,
		Transport:      ft,
	})
	if err != nil {
		return nil, err
	}
	e.link = link

	return e, nil
}

// startAgent 只启动 agentlink（轮询/上报），不启动合成流量生成器。
// 数据面 gateway 子命令复用它；run/loadtest 仍走 start()。
func (e *engine) startAgent(ctx context.Context) error {
	if e.link == nil {
		return fmt.Errorf("agentlink 未初始化")
	}
	if err := e.link.Start(ctx); err != nil {
		return err
	}
	if e.cfg.Template.FetchOnMissing && len(missingTemplates(e.state.get().Policies, e.cache)) > 0 {
		go func() {
			if err := e.link.RefreshTemplates(ctx); err != nil {
				e.log("警告：拉模板失败：%v（将使用内置兜底页）", err)
			}
		}()
	}
	return nil
}

// start 启动所有后台组件。deadline 为零值表示一直运行。
func (e *engine) start(ctx context.Context, deadline time.Time) {
	if e.link != nil {
		if err := e.startAgent(ctx); err != nil {
			e.log("启动 agentlink 失败：%v", err)
		}
	}

	if e.dryRun {
		e.gen.emit = func(Event) { e.st.Enqueued.Add(1) }
	} else {
		e.gen.emit = func(ev Event) {
			if e.link.Enqueue(toAgentEvent(ev)) {
				e.st.Enqueued.Add(1)
			}
		}
	}

	tokens := make(chan struct{}, 8192)
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.gen.pumpTokens(ctx, tokens)
	}()

	e.gen.run(ctx, tokens, deadline)
}

// wait 等待所有后台组件退出。
func (e *engine) wait() {
	e.wg.Wait()
	if e.link != nil {
		e.link.Wait()
	}
}

func (e *engine) linkStats() agentlink.Stats {
	if e.link == nil {
		return agentlink.Stats{
			UploadLatency: emptyPercentiles(),
			PollLatency:   emptyPercentiles(),
		}
	}
	return e.link.Stats()
}

func (e *engine) queueDepth() int {
	if e.link == nil {
		return 0
	}
	return e.link.QueueDepth()
}

func emptyPercentiles() map[string]int64 {
	return map[string]int64{"p50": 0, "p95": 0, "p99": 0, "max": 0}
}

// setDown 运行时切换「shield-x 不可达」，用于故障演练。
func (e *engine) setDown(v bool) { e.ft.SetDown(v) }
