package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControlAPI(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)
	t.Cleanup(func() { _ = c.Stop() })
	mux := newControlMux(c, "")

	do := func(method, path, body string, header map[string]string) (*httptest.ResponseRecorder, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec, rec.Body.String()
	}

	rec, body := do(http.MethodGet, "/api/status", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(body, `"running":false`) {
		t.Fatalf("status = %d %s", rec.Code, body)
	}

	rec, body = do(http.MethodPost, "/api/start", `{"mode":"run","rate":50}`, map[string]string{"Content-Type": "application/json"})
	if rec.Code != http.StatusAccepted || !strings.Contains(body, `"running":true`) {
		t.Fatalf("start = %d %s", rec.Code, body)
	}

	rec, _ = do(http.MethodPost, "/api/start", `{"mode":"run"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复 start = %d，期望 409", rec.Code)
	}

	rec, body = do(http.MethodGet, "/api/config", "", nil)
	if rec.Code != http.StatusOK || strings.Contains(body, "sx_test") {
		t.Fatalf("config 泄漏 Token 或状态错误: %d %s", rec.Code, body)
	}
	if !strings.Contains(body, `"token_set":true`) {
		t.Fatalf("config 缺少 token_set: %s", body)
	}

	rec, body = do(http.MethodPost, "/api/stop", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop = %d %s", rec.Code, body)
	}

	rec, _ = do(http.MethodPost, "/api/stop", "", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("空闲 stop = %d，期望 409", rec.Code)
	}

	rec, body = do(http.MethodPost, "/api/fault", `{"down":true}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(body, `"down":true`) {
		t.Fatalf("fault = %d %s", rec.Code, body)
	}
}

func TestControlAuth(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)
	mux := newControlMux(c, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 Token = %d，期望 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("X-Mock-Token", "secret")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确 Token = %d，期望 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d，期望 200", rec.Code)
	}
}
