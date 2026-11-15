package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrAlreadyRunning 表示已有任务在运行。
	ErrAlreadyRunning = errors.New("controller: 已有任务在运行")
	// ErrNotRunning 表示当前没有任务。
	ErrNotRunning = errors.New("controller: 当前没有任务")
	// ErrInvalidMode 表示不支持的模式。
	ErrInvalidMode = errors.New("controller: 模式只支持 run | loadtest")
)

// Status 是控制台读取的运行状态。
type Status struct {
	Running       bool    `json:"running"`
	Mode          string  `json:"mode,omitempty"`
	Rate          int     `json:"rate"`
	StartedAt     string  `json:"started_at,omitempty"`
	StoppedAt     string  `json:"stopped_at,omitempty"`
	UptimeSeconds float64 `json:"uptime_seconds"`

	Generated   int64 `json:"generated"`
	Enqueued    int64 `json:"enqueued"`
	QueueDepth  int   `json:"queue_depth"`
	SpoolEvents int   `json:"spool_events"`
	SpoolFiles  int   `json:"spool_files"`
	SpoolBytes  int64 `json:"spool_bytes"`

	Sent     uint64 `json:"sent"`
	Accepted uint64 `json:"accepted"`
	Rejected uint64 `json:"rejected"`
	Dropped  uint64 `json:"dropped"`
	Retried  uint64 `json:"retried"`
	Spooled  uint64 `json:"spooled"`
	Replayed uint64 `json:"replayed"`

	PollSuccess     uint64 `json:"poll_success"`
	PollNotModified uint64 `json:"poll_not_modified"`
	PollFailures    uint64 `json:"poll_failures"`
	Revision        int64  `json:"revision"`
	Policies        int    `json:"policies"`
	Templates       int    `json:"templates"`

	LastError string `json:"last_error,omitempty"`
	Down      bool   `json:"down"`
}

// LogEntry 是环形日志的一条记录。
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type ringLogs struct {
	mu      sync.Mutex
	entries []LogEntry
	max     int
}

func newRingLogs(max int) *ringLogs {
	if max < 1 {
		max = 1
	}
	return &ringLogs{max: max}
}

func (r *ringLogs) add(level, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, LogEntry{Time: time.Now(), Level: level, Message: msg})
	if len(r.entries) > r.max {
		copy(r.entries, r.entries[len(r.entries)-r.max:])
		r.entries = r.entries[:r.max]
	}
}

func (r *ringLogs) list(limit int) []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > len(r.entries) {
		limit = len(r.entries)
	}
	out := make([]LogEntry, limit)
	copy(out, r.entries[len(r.entries)-limit:])
	return out
}

// Controller 以可启停的方式管理一次 mock 运行。
type Controller struct {
	cfg     *Config
	baseLog logger
	log     logger
	logs    *ringLogs

	mu        sync.Mutex
	running   bool
	mode      string
	rate      int
	startedAt time.Time
	stoppedAt time.Time
	cancel    context.CancelFunc
	done      chan struct{}
	eng       *engine
	st        *stats
	lastErr   error
	down      bool
}

// NewController 创建 Controller。
func NewController(cfg *Config, base logger) *Controller {
	c := &Controller{
		cfg:     cfg,
		baseLog: base,
		logs:    newRingLogs(500),
		down:    cfg.Fault.ShieldXDown,
	}
	c.log = func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		c.logs.add("info", msg)
		if base != nil {
			base("%s", msg)
		}
	}
	return c
}

// Start 启动一次运行；返回后不阻塞。
func (c *Controller) Start(mode string, rate int, duration time.Duration) error {
	if mode == "" {
		mode = "run"
	}
	if mode != "run" && mode != "loadtest" {
		return ErrInvalidMode
	}
	if rate < 0 {
		return fmt.Errorf("controller: rate 不能为负")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return ErrAlreadyRunning
	}

	cfgCopy := *c.cfg
	if rate > 0 {
		cfgCopy.Generate.Rate = rate
	}

	st := &stats{}
	eng, err := newEngine(&cfgCopy, st, c.log, false)
	if err != nil {
		return err
	}

	deadline := time.Time{}
	if mode == "loadtest" {
		if duration <= 0 {
			duration = 60 * time.Second
		}
		deadline = time.Now().Add(duration)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	c.running = true
	c.mode = mode
	c.rate = cfgCopy.Generate.Rate
	c.startedAt = time.Now()
	c.stoppedAt = time.Time{}
	c.cancel = cancel
	c.done = done
	c.eng = eng
	c.st = st
	c.lastErr = nil

	go func() {
		defer close(done)
		eng.start(ctx, deadline)
		if !deadline.IsZero() {
			timer := time.NewTimer(time.Until(deadline))
			select {
			case <-ctx.Done():
			case <-timer.C:
				cancel()
			}
			timer.Stop()
		}
		eng.wait()

		c.mu.Lock()
		c.running = false
		c.stoppedAt = time.Now()
		if ls := eng.linkStats(); ls.LastError != nil {
			c.lastErr = ls.LastError
		}
		c.mu.Unlock()
	}()

	return nil
}

// Stop 停止当前运行并等待后台 flush 完成。
func (c *Controller) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return ErrNotRunning
	}
	cancel := c.cancel
	done := c.done
	c.mu.Unlock()

	cancel()
	select {
	case <-done:
		return nil
	case <-time.After(15 * time.Second):
		return errors.New("controller: 停止超时（后台仍有未完成的 flush）")
	}
}

// SetDown 切换「shield-x 不可达」。
func (c *Controller) SetDown(v bool) {
	c.mu.Lock()
	c.down = v
	eng := c.eng
	c.mu.Unlock()
	if eng != nil {
		eng.setDown(v)
	}
}

// Status 返回当前状态快照。
func (c *Controller) Status() Status {
	c.mu.Lock()
	running := c.running
	mode := c.mode
	rate := c.rate
	started := c.startedAt
	stopped := c.stoppedAt
	eng := c.eng
	st := c.st
	lastErr := c.lastErr
	down := c.down
	c.mu.Unlock()

	out := Status{Running: running, Mode: mode, Rate: rate, Down: down}
	if !started.IsZero() {
		out.StartedAt = started.Format(time.RFC3339)
		if running {
			out.UptimeSeconds = time.Since(started).Seconds()
		} else if !stopped.IsZero() {
			out.UptimeSeconds = stopped.Sub(started).Seconds()
		}
	}
	if !stopped.IsZero() {
		out.StoppedAt = stopped.Format(time.RFC3339)
	}
	if st != nil {
		out.Generated = st.Generated.Load()
		out.Enqueued = st.Enqueued.Load()
	}
	if eng != nil {
		ls := eng.linkStats()
		sn := eng.state.get()
		out.QueueDepth = ls.QueueDepth
		out.SpoolEvents = ls.SpoolEvents
		out.SpoolFiles = ls.SpoolFiles
		out.SpoolBytes = ls.SpoolBytes
		out.Sent = ls.Sent
		out.Accepted = ls.Accepted
		out.Rejected = ls.Rejected
		out.Dropped = ls.Dropped
		out.Retried = ls.Retried
		out.Spooled = ls.Spooled
		out.Replayed = ls.Replayed
		out.PollSuccess = ls.PollSuccess
		out.PollNotModified = ls.PollNotModified
		out.PollFailures = ls.PollFailures
		out.Revision = sn.Revision
		out.Policies = len(sn.Policies)
		out.Templates = eng.cache.len()
		if ls.LastError != nil {
			lastErr = ls.LastError
		}
	}
	if lastErr != nil {
		out.LastError = lastErr.Error()
	}
	return out
}

// Logs 返回最近 limit 条日志。
func (c *Controller) Logs(limit int) []LogEntry { return c.logs.list(limit) }

// Policies 返回当前策略快照。
func (c *Controller) Policies() snapshot {
	c.mu.Lock()
	eng := c.eng
	c.mu.Unlock()
	if eng == nil {
		return snapshot{}
	}
	return eng.state.get()
}

// Templates 返回当前模板缓存。
func (c *Controller) Templates() []Template {
	c.mu.Lock()
	eng := c.eng
	c.mu.Unlock()
	if eng == nil {
		return nil
	}
	return eng.cache.snapshot()
}
