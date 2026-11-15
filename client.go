package main

import (
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// faultTransport 在真实 Transport 外面包一层故障注入，
// 用来验证 shield-x 侧或网络异常时的客户端行为。
type faultTransport struct {
	base http.RoundTripper
	cfg  FaultConfig

	down atomic.Bool
	mu   sync.Mutex
	rnd  *rand.Rand
}

func newFaultTransport(cfg FaultConfig) *faultTransport {
	src := rand.NewSource(time.Now().UnixNano())
	return &faultTransport{
		base: http.DefaultTransport,
		cfg:  cfg,
		rnd:  rand.New(src),
	}
}

// SetDown 运行时切换「shield-x 不可达」。
func (t *faultTransport) SetDown(v bool) { t.down.Store(v) }

func (t *faultTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.cfg.ShieldXDown || t.down.Load() {
		return nil, errors.New("fault: shield-x 不可达（注入）")
	}

	if t.cfg.Latency.D() > 0 {
		time.Sleep(t.cfg.Latency.D())
	}

	if t.cfg.DropPercent > 0 {
		t.mu.Lock()
		drop := t.rnd.Intn(100) < t.cfg.DropPercent
		t.mu.Unlock()
		if drop {
			return nil, errors.New("fault: 随机丢包（注入）")
		}
	}

	if t.cfg.HTTPStatusOverride != 0 {
		return &http.Response{
			StatusCode: t.cfg.HTTPStatusOverride,
			Status:     http.StatusText(t.cfg.HTTPStatusOverride),
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}

	return t.base.RoundTrip(req)
}
