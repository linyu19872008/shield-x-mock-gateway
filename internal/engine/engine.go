// Package engine wires the SDK agent link, policy state, templates, and traffic generator.
package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/linyu19872008/shield-x-client-go/agent"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/agentlink"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/config"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/contract"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/generator"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/logging"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/state"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/template"
)

// Engine connects the SDK agent link to the local policy state and generator.
type Engine struct {
	cfg    *config.Config
	st     *state.Stats
	log    logging.Logger
	dryRun bool

	ft    *faultTransport
	state *state.State
	cache *template.Cache
	link  *agentlink.Link
	gen   *generator.Generator

	wg sync.WaitGroup
}

// New creates an engine. It loads the persisted snapshot before the first poll.
func New(cfg *config.Config, st *state.Stats, log logging.Logger, dryRun bool) (*Engine, error) {
	policyState := &state.State{}
	cache := template.NewCache()
	ft := newFaultTransport(cfg.Fault)

	gen := generator.New(cfg, policyState, cache, st)
	gen.InitBackfill()

	e := &Engine{
		cfg:    cfg,
		st:     st,
		log:    log,
		dryRun: dryRun,
		ft:     ft,
		state:  policyState,
		cache:  cache,
		gen:    gen,
	}
	if dryRun {
		return e, nil
	}

	initial := state.Load(cfg, policyState, log)

	var link *agentlink.Link
	link, err := agentlink.New(agentlink.Options{
		BaseURL:         cfg.ShieldX.BaseURL,
		Token:           cfg.ShieldX.Token,
		GatewayID:       cfg.ShieldX.GatewayID,
		AgentVersion:    cfg.ShieldX.AgentVersion,
		Capabilities:    agentlink.ToAgentCapabilities(cfg.ShieldX.Capabilities),
		SigningKey:      cfg.ShieldX.SigningKey,
		PollInterval:    cfg.Poll.Interval.D(),
		PollTimeout:     cfg.Poll.Timeout.D(),
		InitialRevision: initial,
		OnPolicies: func(s *agent.PolicySnapshot) error {
			snap := agentlink.ToSnapshot(s, cfg.ShieldX.BaseURL)
			policyState.Apply(snap)
			state.Save(cfg, snap, log)
			if cfg.Template.FetchOnMissing && len(template.MissingTemplates(snap.Policies, cache)) > 0 {
				go func() {
					if err := link.RefreshTemplates(context.Background()); err != nil {
						log("警告：拉模板失败：%v（将使用内置兜底页）", err)
					}
				}()
			}
			return nil
		},
		OnTemplates: func(s *agent.TemplateSnapshot) {
			cache.Replace(agentlink.ToTemplates(s))
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

// StartAgent starts only the SDK poller/reporter. The data plane uses this;
// run/loadtest use Start instead.
func (e *Engine) StartAgent(ctx context.Context) error {
	if e.link == nil {
		return fmt.Errorf("agentlink 未初始化")
	}
	if err := e.link.Start(ctx); err != nil {
		return err
	}
	if e.cfg.Template.FetchOnMissing && len(template.MissingTemplates(e.state.Get().Policies, e.cache)) > 0 {
		go func() {
			if err := e.link.RefreshTemplates(ctx); err != nil {
				e.log("警告：拉模板失败：%v（将使用内置兜底页）", err)
			}
		}()
	}
	return nil
}

// Start starts the agent link and the synthetic generator. A zero deadline runs forever.
func (e *Engine) Start(ctx context.Context, deadline time.Time) {
	if e.link != nil {
		if err := e.StartAgent(ctx); err != nil {
			e.log("启动 agentlink 失败：%v", err)
		}
	}

	if e.dryRun {
		e.gen.SetEmit(func(contract.Event) { e.st.Enqueued.Add(1) })
	} else {
		e.gen.SetEmit(func(ev contract.Event) {
			if e.Enqueue(ev) {
				e.st.Enqueued.Add(1)
			}
		})
	}

	tokens := make(chan struct{}, 8192)
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.gen.PumpTokens(ctx, tokens)
	}()

	e.gen.Run(ctx, tokens, deadline)
}

// Wait waits for all background components to exit.
func (e *Engine) Wait() {
	e.wg.Wait()
	if e.link != nil {
		e.link.Wait()
	}
}

// WaitGenerator waits for the synthetic generator to finish.
func (e *Engine) WaitGenerator() { e.gen.Wait() }

// LinkStats returns the SDK poller/reporter statistics.
func (e *Engine) LinkStats() agentlink.Stats {
	if e.link == nil {
		return agentlink.Stats{
			UploadLatency: emptyPercentiles(),
			PollLatency:   emptyPercentiles(),
		}
	}
	return e.link.Stats()
}

// QueueDepth returns the reporter queue depth.
func (e *Engine) QueueDepth() int {
	if e.link == nil {
		return 0
	}
	return e.link.QueueDepth()
}

// State returns the policy state shared with the data plane.
func (e *Engine) State() *state.State { return e.state }

// Templates returns the template cache shared with the data plane.
func (e *Engine) Templates() *template.Cache { return e.cache }

// Enqueue converts and queues one mock event.
func (e *Engine) Enqueue(ev contract.Event) bool {
	if e.link == nil {
		return false
	}
	return e.link.Enqueue(agentlink.ToAgentEvent(ev))
}

// SetDown toggles the runtime "shield-x unreachable" fault.
func (e *Engine) SetDown(v bool) { e.ft.SetDown(v) }

func emptyPercentiles() map[string]int64 {
	return map[string]int64{"p50": 0, "p95": 0, "p99": 0, "max": 0}
}
