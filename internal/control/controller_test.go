package control

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/config"
	"time"
)

func newTestControlConfig(t *testing.T) *config.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shieldx/policies/v1":
			w.Header().Set("ETag", `"rev-1"`)
			_, _ = w.Write([]byte(`{"revision":1,"generated_at":"2026-11-14T00:00:00Z","policy_count":0,"filtered_out":0,"policies":[]}`))
		case "/shieldx/events/v1":
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"accepted":1,"rejected":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.ShieldX.BaseURL = srv.URL + "/shieldx"
	cfg.ShieldX.Token = "sx_test"
	cfg.ShieldX.GatewayID = "gw-test"
	cfg.ShieldX.AgentVersion = "mock-test"
	cfg.Poll.PersistPath = ""
	cfg.Report.SpoolDir = ""
	cfg.Generate.Rate = 50
	cfg.Generate.Duration = 0
	return cfg
}

func TestControllerStartStop(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)

	if err := c.Start("run", 50, 0); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if !c.Status().Running {
		t.Fatal("启动后 Running 应为 true")
	}
	if err := c.Start("run", 50, 0); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("重复启动错误 = %v，期望 ErrAlreadyRunning", err)
	}

	time.Sleep(150 * time.Millisecond)

	if err := c.Stop(); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	st := c.Status()
	if st.Running {
		t.Fatal("停止后 Running 应为 false")
	}
	if st.Generated == 0 {
		t.Fatal("运行期间应生成事件")
	}
	if err := c.Stop(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("空闲 Stop 错误 = %v，期望 ErrNotRunning", err)
	}
}

func TestControllerModeValidation(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)

	if err := c.Start("bad-mode", 0, 0); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("错误 = %v，期望 ErrInvalidMode", err)
	}
	if err := c.Start("run", -1, 0); err == nil {
		t.Fatal("负 rate 应报错")
	}
}

func TestControllerSetDown(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)
	c.SetDown(true)
	if !c.Status().Down {
		t.Fatal("Down 应为 true")
	}
	c.SetDown(false)
	if c.Status().Down {
		t.Fatal("Down 应为 false")
	}
}
