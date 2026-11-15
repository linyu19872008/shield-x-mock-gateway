package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedUI(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)
	h, err := newServeHandler(c, "")
	if err != nil {
		t.Fatalf("newServeHandler 失败: %v", err)
	}

	for _, path := range []string{"/", "/app.css", "/app.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d", path, rec.Code)
		}
		if path == "/" && !strings.Contains(rec.Body.String(), "mock-gateway") {
			t.Fatal("首页未包含产品名")
		}
		if path == "/app.js" && !strings.Contains(rec.Body.String(), "refreshStatus") {
			t.Fatal("app.js 内容异常")
		}
	}
}

func TestEmbeddedUIAuth(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)
	h, err := newServeHandler(c, "secret")
	if err != nil {
		t.Fatalf("newServeHandler 失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("静态首页应无需 Token，实际 %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("API 无 Token 状态码 = %d，期望 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("X-Mock-Token", "secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("API 带 Token 状态码 = %d，期望 200", rec.Code)
	}
}

func TestPoliciesAndTemplatesAPI(t *testing.T) {
	cfg := newTestControlConfig(t)
	c := NewController(cfg, nil)
	h, err := newServeHandler(c, "")
	if err != nil {
		t.Fatalf("newServeHandler 失败: %v", err)
	}

	for _, path := range []string{"/api/policies", "/api/templates", "/api/config"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d (%s)", path, rec.Code, rec.Body.String())
		}
	}
}
