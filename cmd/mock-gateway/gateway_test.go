package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGatewayTestHandler(policies []Policy, revision int64) (*gatewayHandler, *[]Event) {
	cfg := defaultConfig()
	cfg.Ingress.MockStatus = 200
	cfg.Ingress.MockBody = "mock upstream\n"
	h := &gatewayHandler{
		cfg:   cfg,
		state: &policyState{},
		cache: newTemplateCache(),
		log:   func(string, ...any) {},
	}
	h.state.apply(snapshot{Revision: revision, Policies: policies})
	events := &[]Event{}
	h.enqueue = func(ev Event) bool {
		*events = append(*events, ev)
		return true
	}
	return h, events
}

func doGatewayRequest(h *gatewayHandler, method, target, ua, remote string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGatewayRedirect302(t *testing.T) {
	p := Policy{
		ID:       "pol_redirect",
		Priority: 850,
		Match:    json.RawMessage(`{"all":[{"field":"ua","op":"contains","value":"Ahrefs"}]}`),
		Action: Action{Type: "redirect", Params: map[string]any{
			"redirect_url": "https://www.baidu.com",
			"status_code":  float64(302),
		}},
	}
	h, events := newGatewayTestHandler([]Policy{p}, 12)
	w := doGatewayRequest(h, http.MethodGet, "http://api.example.com/crawler-test", "AhrefsBot/7.0", "198.51.100.10:1234")

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if got := w.Header().Get("Location"); got != "https://www.baidu.com" {
		t.Fatalf("Location = %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("redirect should have an empty body, got %q", w.Body.String())
	}
	if len(*events) != 1 {
		t.Fatalf("events = %d, want 1", len(*events))
	}
	ev := (*events)[0]
	if ev.Action != "redirect" || ev.Status != 302 || ev.ActionResult != "executed" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.MatchedPolicyID == nil || *ev.MatchedPolicyID != "pol_redirect" {
		t.Fatalf("matched policy missing: %+v", ev)
	}
	if ev.ConfigRevision != 12 {
		t.Fatalf("revision = %d, want 12", ev.ConfigRevision)
	}
}

func TestGatewayNoPolicyUsesMockUpstream(t *testing.T) {
	h, events := newGatewayTestHandler(nil, 12)
	w := doGatewayRequest(h, http.MethodGet, "http://api.example.com/ok", "Mozilla/5.0", "198.51.100.10:1234")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "mock upstream\n" {
		t.Fatalf("body = %q", got)
	}
	if len(*events) != 1 {
		t.Fatalf("events = %d, want 1", len(*events))
	}
	ev := (*events)[0]
	if ev.Action != "none" || ev.DecisionSource != "none" || ev.Status != 200 {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestGatewayBlockFallsBackToBuiltinTemplate(t *testing.T) {
	p := Policy{
		ID:       "pol_block",
		Priority: 1000,
		Match:    json.RawMessage(`{"all":[{"field":"path","op":"prefix","value":"/.env"}]}`),
		Action:   Action{Type: "block"},
	}
	h, events := newGatewayTestHandler([]Policy{p}, 7)
	w := doGatewayRequest(h, http.MethodGet, "http://api.example.com/.env", "curl/8.4.0", "198.51.100.10:1234")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "访问被拦截") {
		t.Fatalf("builtin block page missing: %q", w.Body.String())
	}
	if w.Header().Get("X-Shieldx-Request-Id") == "" {
		t.Fatal("missing X-Shieldx-Request-Id")
	}
	ev := (*events)[0]
	if ev.Action != "block" || ev.ActionResult != "failed" {
		t.Fatalf("fallback block should be failed, got %+v", ev)
	}
	if ev.BlockTemplateID != nil {
		t.Fatalf("builtin fallback should not set a template ID: %+v", ev)
	}
}

func TestGatewayEvaluationErrorIsGatewayError(t *testing.T) {
	p := Policy{
		ID:       "pol_bad",
		Priority: 1,
		Match:    json.RawMessage(`{"all":[{"field":"bogus","op":"eq","value":"x"}]}`),
		Action:   Action{Type: "observe"},
	}
	h, events := newGatewayTestHandler([]Policy{p}, 3)
	w := doGatewayRequest(h, http.MethodGet, "http://api.example.com/", "curl/8.4.0", "198.51.100.10:1234")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	ev := (*events)[0]
	if ev.DecisionSource != "gateway_error" || ev.ActionResult != "failed" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestGatewayTrustProxyOnlyWhenEnabled(t *testing.T) {
	p := Policy{
		ID:       "pol_ip",
		Priority: 900,
		Match:    json.RawMessage(`{"all":[{"field":"ip","op":"in_cidr","value":["203.0.113.0/24"]}]}`),
		Action:   Action{Type: "observe"},
	}
	h, events := newGatewayTestHandler([]Policy{p}, 1)
	r := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
	r.RemoteAddr = "198.51.100.10:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.66")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if got := (*events)[0].Action; got != "none" {
		t.Fatalf("untrusted XFF should not match, action=%q", got)
	}

	h.cfg.Ingress.TrustProxy = true
	events2 := &[]Event{}
	h.enqueue = func(ev Event) bool { *events2 = append(*events2, ev); return true }
	rec = httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
	r2.RemoteAddr = "198.51.100.10:1234"
	r2.Header.Set("X-Forwarded-For", "203.0.113.66")
	h.ServeHTTP(rec, r2)
	if got := (*events2)[0].Action; got != "observe" {
		t.Fatalf("trusted XFF should match, action=%q", got)
	}
}

func TestGatewayDoesNotReadUpstream(t *testing.T) {
	// 仅确认 handler 不会发起任何请求：请求上下文里没有可用的业务地址，
	// 且响应完全由 mock 生成。该测试的断言是 body 来自配置，而不是上游。
	h, _ := newGatewayTestHandler(nil, 1)
	w := doGatewayRequest(h, http.MethodGet, "http://not-a-real-upstream.invalid/", "curl/8.4.0", "198.51.100.10:1234")
	body, _ := io.ReadAll(w.Result().Body)
	if string(body) != "mock upstream\n" {
		t.Fatalf("body = %q", string(body))
	}
}
