// Package agentlink 是 mock-gateway 与 shield-x-client-go 之间的唯一适配层。
//
// mock 的求值、模板渲染、流量生成保持独立；所有 agent 侧 HTTP 调用
// （拉策略、拉模板、上报事件、spool 补发）都经由本包。
package agentlink

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/linyu19872008/shield-x-client-go/agent"
	"github.com/linyu19872008/shield-x-client-go/poller"
	"github.com/linyu19872008/shield-x-client-go/reporter"
	"github.com/linyu19872008/shield-x-client-go/reporter/spool"
)

// Options 是 agentlink 的配置。由 mock 的 Config 映射而来。
type Options struct {
	BaseURL      string
	Token        string
	GatewayID    string
	AgentVersion string
	Capabilities agent.Capabilities
	SigningKey   string

	PollInterval    time.Duration
	PollTimeout     time.Duration
	InitialRevision int64
	FetchTemplates  bool
	OnPolicies      func(*agent.PolicySnapshot) error
	OnTemplates     func(*agent.TemplateSnapshot)
	OnError         func(error)

	ReportInterval time.Duration
	BatchSize      int
	Format         agent.Format
	DisableGzip    bool
	ReportTimeout  time.Duration
	MaxBackoff     time.Duration

	SpoolDir   string
	SpoolMaxMB int

	Transport http.RoundTripper
}

// Stats 是 poller + reporter + 延迟统计的合并快照。
type Stats struct {
	QueueDepth  int
	SpoolEvents int
	SpoolFiles  int
	SpoolBytes  int64

	Sent     uint64
	Accepted uint64
	Rejected uint64
	Dropped  uint64
	Spooled  uint64
	Replayed uint64
	Retried  uint64

	PollSuccess     uint64
	PollNotModified uint64
	PollFailures    uint64
	LastRevision    int64
	LastError       error

	UploadLatency map[string]int64
	PollLatency   map[string]int64
}

// Link 持有 SDK 客户端与后台组件。
type Link struct {
	opts     Options
	client   *agent.Client
	poller   *poller.Poller
	reporter *reporter.Reporter
	disk     *spool.Disk
	timing   *timingTransport

	mu          sync.Mutex
	revision    int64
	policyCount int
	running     bool

	wg sync.WaitGroup
}

// New 创建 Link。
func New(opts Options) (*Link, error) {
	if opts.SpoolMaxMB <= 0 {
		opts.SpoolMaxMB = 200
	}

	base := opts.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	timing := &timingTransport{base: base}

	l := &Link{opts: opts, timing: timing, revision: opts.InitialRevision}

	var caps *agent.Capabilities
	if len(opts.Capabilities.Actions) > 0 {
		caps = &opts.Capabilities
	}

	client, err := agent.New(agent.Config{
		BaseURL:       opts.BaseURL,
		Token:         opts.Token,
		GatewayID:     opts.GatewayID,
		AgentVersion:  opts.AgentVersion,
		Capabilities:  caps,
		SigningKey:    opts.SigningKey,
		Transport:     timing,
		FetchTimeout:  opts.PollTimeout,
		ReportTimeout: opts.ReportTimeout,
	})
	if err != nil {
		return nil, err
	}
	l.client = client

	var sp reporter.Spool
	if opts.SpoolDir != "" {
		disk, err := spool.NewDisk(opts.SpoolDir, spool.WithMaxBytes(int64(opts.SpoolMaxMB)<<20))
		if err != nil {
			return nil, err
		}
		l.disk = disk
		sp = disk
	}

	r, err := reporter.New(client, reporter.Options{
		BatchSize:     opts.BatchSize,
		FlushInterval: opts.ReportInterval,
		Format:        opts.Format,
		DisableGzip:   opts.DisableGzip,
		Timeout:       opts.ReportTimeout,
		MaxBackoff:    opts.MaxBackoff,
		Spool:         sp,
		State:         l.state,
		OnError:       l.onError,
	})
	if err != nil {
		return nil, err
	}
	l.reporter = r

	p, err := poller.New(client, poller.Options{
		Interval:        opts.PollInterval,
		Timeout:         opts.PollTimeout,
		InitialRevision: opts.InitialRevision,
		OnSnapshot:      l.onPolicies,
		OnNotModified:   l.onNotModified,
		OnError:         l.onError,
		FetchTemplates:  opts.FetchTemplates,
		OnTemplates:     opts.OnTemplates,
	})
	if err != nil {
		return nil, err
	}
	l.poller = p

	return l, nil
}

// Start 启动 poller 与 reporter；返回后不阻塞。
func (l *Link) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return errors.New("agentlink: 已在运行")
	}
	l.running = true
	l.mu.Unlock()

	l.wg.Add(2)
	go func() {
		defer l.wg.Done()
		_ = l.poller.Run(ctx)
	}()
	go func() {
		defer l.wg.Done()
		_ = l.reporter.Run(ctx)
	}()
	return nil
}

// Wait 等待后台组件退出。
func (l *Link) Wait() { l.wg.Wait() }

// Enqueue 入队一个事件。
func (l *Link) Enqueue(ev agent.Event) bool { return l.reporter.Enqueue(ev) }

// RefreshTemplates 手动拉取一次模板。
func (l *Link) RefreshTemplates(ctx context.Context) error {
	return l.poller.RefreshTemplates(ctx)
}

// QueueDepth 返回上报队列深度。
func (l *Link) QueueDepth() int { return l.reporter.Stats().QueueDepth }

// Stats 返回合并统计。
func (l *Link) Stats() Stats {
	rs := l.reporter.Stats()
	ps := l.poller.Stats()
	upload, pollLat := l.timing.percentiles()

	l.mu.Lock()
	revision := l.revision
	l.mu.Unlock()
	if ps.LastRevision != 0 {
		revision = ps.LastRevision
	}

	lastErr := rs.LastError
	if lastErr == nil {
		lastErr = ps.LastError
	}

	spoolEvents := 0
	spoolFiles := rs.SpoolFiles
	spoolBytes := rs.SpoolBytes
	if l.disk != nil {
		ss := l.disk.Stats()
		spoolEvents = ss.Events
		spoolFiles = ss.Files
		spoolBytes = ss.Bytes
	}

	return Stats{
		QueueDepth:      rs.QueueDepth,
		SpoolEvents:     spoolEvents,
		SpoolFiles:      spoolFiles,
		SpoolBytes:      spoolBytes,
		Sent:            rs.Sent,
		Accepted:        rs.Accepted,
		Rejected:        rs.Rejected,
		Dropped:         rs.Dropped,
		Spooled:         rs.Spooled,
		Replayed:        rs.Replayed,
		Retried:         rs.Retried,
		PollSuccess:     ps.Success,
		PollNotModified: ps.NotModified,
		PollFailures:    ps.Failures,
		LastRevision:    revision,
		LastError:       lastErr,
		UploadLatency:   upload,
		PollLatency:     pollLat,
	}
}

func (l *Link) state() (int64, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.revision, l.policyCount
}

func (l *Link) onPolicies(s *agent.PolicySnapshot) {
	l.mu.Lock()
	l.revision = s.Revision
	l.policyCount = len(s.Policies)
	l.mu.Unlock()
	if l.opts.OnPolicies != nil {
		if err := l.opts.OnPolicies(s); err != nil {
			_ = l.client.ReportApplyStatus(context.Background(), agent.ApplyStatusRequest{
				AppliedRevision: s.Revision,
				Status:          "failed",
				ErrorCode:       "APPLY_FAILED",
				ErrorDetail:     err.Error(),
			})
			return
		}
	}
	_ = l.client.ReportApplyStatus(context.Background(), agent.ApplyStatusRequest{
		AppliedRevision: s.Revision,
		Status:          "applied",
		PolicyCount:     len(s.Policies),
	})
}

func (l *Link) onNotModified(revision int64) {
	l.mu.Lock()
	l.revision = revision
	l.mu.Unlock()
}

func (l *Link) onError(err error) {
	if err != nil && l.opts.OnError != nil {
		l.opts.OnError(err)
	}
}

// timingTransport 记录 events / policies 请求耗时，供压测报告使用。
type timingTransport struct {
	base http.RoundTripper

	mu     sync.Mutex
	upload []int64
	poll   []int64
}

func (t *timingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	ms := time.Since(start).Milliseconds()

	t.mu.Lock()
	switch {
	case strings.Contains(req.URL.Path, "/events/"):
		t.upload = appendCapped(t.upload, ms)
	case strings.Contains(req.URL.Path, "/policies/"), strings.Contains(req.URL.Path, "/templates/"):
		t.poll = appendCapped(t.poll, ms)
	}
	t.mu.Unlock()

	return resp, err
}

func (t *timingTransport) percentiles() (map[string]int64, map[string]int64) {
	t.mu.Lock()
	upload := append([]int64{}, t.upload...)
	poll := append([]int64{}, t.poll...)
	t.mu.Unlock()
	return percentiles(upload), percentiles(poll)
}

func appendCapped(values []int64, v int64) []int64 {
	const maxSamples = 200000
	if len(values) >= maxSamples {
		return values
	}
	return append(values, v)
}

func percentiles(values []int64) map[string]int64 {
	out := map[string]int64{"p50": 0, "p95": 0, "p99": 0, "max": 0}
	if len(values) == 0 {
		return out
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(p float64) int64 {
		idx := int(float64(len(sorted)-1) * p)
		return sorted[idx]
	}
	out["p50"] = pick(0.50)
	out["p95"] = pick(0.95)
	out["p99"] = pick(0.99)
	out["max"] = sorted[len(sorted)-1]
	return out
}
