package dataplane

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/config"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/contract"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/evaluator"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/logging"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/state"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/template"
)

// gatewayHandler 是 mock-gateway 的数据面：接收真实请求、本地求值，
// 返回 mock 响应并上报真实事件。它不代理、不访问任何业务系统。
type Handler struct {
	cfg     *config.Config
	state   *state.State
	cache   *template.Cache
	enqueue func(contract.Event) bool
	log     logging.Logger
	seq     atomic.Int64
}

// gatewayResponse 是一次请求的返回计划。
// NewHandler creates the HTTP data-plane handler. The handler never proxies upstream.
func NewHandler(cfg *config.Config, st *state.State, cache *template.Cache, enqueue func(contract.Event) bool, log logging.Logger) *Handler {
	return &Handler{cfg: cfg, state: st, cache: cache, enqueue: enqueue, log: log}
}

type gatewayResponse struct {
	status         int
	body           []byte
	contentType    string
	headers        map[string]string
	action         string
	actionResult   string
	decisionSource string
	policy         *contract.Policy
	template       *contract.Template
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	eventID := template.RequestID(h.cfg.ShieldX.GatewayID, h.seq.Add(1))

	clientIP := requestClientIP(r, h.cfg.Ingress.TrustProxy)
	evalReq := &evaluator.Request{
		IP:      clientIP,
		Host:    r.Host,
		Path:    r.URL.Path,
		Method:  r.Method,
		UA:      r.UserAgent(),
		Headers: requestHeaders(r),
		Query:   requestQuery(r),
		Cookies: requestCookies(r),
	}

	snap := h.state.Get()
	evalStarted := time.Now()
	policy, evalErr := evaluator.Evaluate(snap.Policies, evalReq)
	evalUS := int(time.Since(evalStarted).Microseconds())

	plan := h.buildResponse(policy, evalErr, eventID, evalReq)
	for k, v := range plan.headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", plan.contentType)
	w.Header().Set("X-Shieldx-Request-Id", eventID)
	w.Header().Set("X-Shieldx-Mock", "dataplane")
	w.WriteHeader(plan.status)
	if len(plan.body) > 0 {
		_, _ = w.Write(plan.body)
	}

	ev := contract.Event{
		EventID:           eventID,
		TS:                started.Format(time.RFC3339Nano),
		GatewayID:         h.cfg.ShieldX.GatewayID,
		ClientIP:          clientIP,
		Host:              r.Host,
		Path:              r.URL.Path,
		Query:             r.URL.RawQuery,
		Method:            r.Method,
		Scheme:            requestScheme(r),
		UA:                r.UserAgent(),
		Referer:           r.Referer(),
		Status:            plan.status,
		DurationMS:        int(time.Since(started).Milliseconds()),
		Bytes:             int64(len(plan.body)),
		ConfigRevision:    snap.Revision,
		Action:            plan.action,
		ActionResult:      plan.actionResult,
		DecisionSource:    plan.decisionSource,
		DecisionLatencyUS: evalUS,
		TraceID:           eventID,
	}
	if plan.policy != nil {
		id := plan.policy.ID
		priority := plan.policy.Priority
		ev.MatchedPolicyID = &id
		ev.MatchedPolicyPriority = &priority
	}
	if plan.template != nil {
		id := plan.template.ID
		version := plan.template.Version
		ev.BlockTemplateID = &id
		ev.BlockTemplateVersion = &version
	}

	if h.enqueue != nil && !h.enqueue(ev) {
		h.log("上报队列已满，事件丢弃：%s", eventID)
	}
}

func (h *Handler) buildResponse(p *contract.Policy, evalErr error, requestID string, req *evaluator.Request) gatewayResponse {
	if evalErr != nil {
		// 策略求值异常必须显式成为 gateway_error，不能伪装成“未命中”。
		return gatewayResponse{
			status:         http.StatusServiceUnavailable,
			body:           []byte("mock gateway: policy evaluation failed\n"),
			contentType:    "text/plain; charset=utf-8",
			headers:        map[string]string{"Cache-Control": "no-store"},
			action:         "none",
			actionResult:   "failed",
			decisionSource: "gateway_error",
		}
	}
	if p == nil {
		return gatewayResponse{
			status:         h.cfg.Ingress.MockStatus,
			body:           []byte(h.cfg.Ingress.MockBody),
			contentType:    h.cfg.Ingress.MockContentType,
			action:         "none",
			actionResult:   "executed",
			decisionSource: "none",
		}
	}

	vars := map[string]string{
		"request_id":    requestID,
		"timestamp":     contract.NowRFC3339(),
		"client_ip":     req.IP,
		"host":          req.Host,
		"path":          req.Path,
		"site_name":     h.cfg.ShieldX.GatewayID,
		"contact_email": "security@example.com",
	}

	switch p.Action.Type {
	case "observe":
		return gatewayResponse{
			status:         h.cfg.Ingress.MockStatus,
			body:           []byte(h.cfg.Ingress.MockBody),
			contentType:    h.cfg.Ingress.MockContentType,
			action:         "observe",
			actionResult:   "executed",
			decisionSource: "policy",
			policy:         p,
		}

	case "block":
		html, tpl, ok := template.RenderForPolicy(h.cache, p, vars)
		status := p.Action.ParamInt("status_code", 0)
		if status == 0 {
			status = tpl.StatusCode
		}
		if status == 0 {
			status = http.StatusForbidden
		}
		headers := map[string]string{"Cache-Control": "no-store"}
		for k, v := range tpl.Headers {
			headers[k] = v
		}
		contentType := tpl.ContentType
		if contentType == "" {
			contentType = "text/html; charset=utf-8"
		}
		result := "executed"
		if !ok {
			result = "failed"
		}
		var templatePtr *contract.Template
		if tpl.ID != "" {
			templatePtr = &tpl
		}
		return gatewayResponse{
			status:         status,
			body:           []byte(html),
			contentType:    contentType,
			headers:        headers,
			action:         "block",
			actionResult:   result,
			decisionSource: "policy",
			policy:         p,
			template:       templatePtr,
		}

	case "redirect":
		status := p.Action.ParamInt("status_code", http.StatusFound)
		if status != http.StatusMovedPermanently && status != http.StatusFound && status != http.StatusTemporaryRedirect {
			status = http.StatusFound
		}
		return gatewayResponse{
			status:         status,
			contentType:    "text/plain; charset=utf-8",
			headers:        map[string]string{"Location": p.Action.ParamString("redirect_url"), "Cache-Control": "no-store"},
			action:         "redirect",
			actionResult:   "executed",
			decisionSource: "policy",
			policy:         p,
		}

	case "javascript_challenge":
		body := []byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>Mock challenge</title></head><body><h1>Mock JS challenge</h1><p>This is a mock challenge page from mock-gateway.</p></body></html>")
		return gatewayResponse{
			status:         http.StatusOK,
			body:           body,
			contentType:    "text/html; charset=utf-8",
			headers:        map[string]string{"Cache-Control": "no-store"},
			action:         "javascript_challenge",
			actionResult:   "challenged",
			decisionSource: "policy",
			policy:         p,
		}

	default:
		return gatewayResponse{
			status:         http.StatusServiceUnavailable,
			body:           []byte("mock gateway: unsupported policy action\n"),
			contentType:    "text/plain; charset=utf-8",
			headers:        map[string]string{"Cache-Control": "no-store"},
			action:         "none",
			actionResult:   "failed",
			decisionSource: "gateway_error",
			policy:         p,
		}
	}
}

func requestClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if net.ParseIP(first) != nil {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestHeaders(r *http.Request) map[string]string {
	out := make(map[string]string, len(r.Header))
	for k, values := range r.Header {
		if len(values) > 0 {
			out[k] = values[0]
		}
	}
	return out
}

func requestQuery(r *http.Request) map[string]string {
	q := r.URL.Query()
	out := make(map[string]string, len(q))
	for k, values := range q {
		if len(values) > 0 {
			out[k] = values[0]
		}
	}
	return out
}

func requestCookies(r *http.Request) map[string]string {
	out := map[string]string{}
	for _, c := range r.Cookies() {
		out[c.Name] = c.Value
	}
	return out
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
