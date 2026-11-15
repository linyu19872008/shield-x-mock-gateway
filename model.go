package main

import (
	"encoding/json"
	"time"
)

// ---------- 契约实体（与 gateway 接入契约 v1 一一对应） ----------

// Policy 是拉策略接口返回的一条策略。
type Policy struct {
	ID       string          `json:"id"`
	Version  int             `json:"version"`
	Name     string          `json:"name"`
	Priority int             `json:"priority"`
	Match    json.RawMessage `json:"match"`
	Action   Action          `json:"action"`
}

// Action 描述命中后执行什么。
type Action struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

// ParamString 读取字符串参数。
func (a Action) ParamString(key string) string {
	if a.Params == nil {
		return ""
	}
	if v, ok := a.Params[key].(string); ok {
		return v
	}
	return ""
}

// ParamInt 读取整型参数（JSON 数字在 Go 里是 float64）。
func (a Action) ParamInt(key string, def int) int {
	if a.Params == nil {
		return def
	}
	switch v := a.Params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// Template 是 GET /shieldx/templates/v1 返回的一条模板。
type Template struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     int               `json:"version"`
	Checksum    string            `json:"checksum"`
	ContentType string            `json:"content_type"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers"`
	Content     string            `json:"content"`
	Vars        map[string]string `json:"vars"`
}

// ---------- 事件上报 ----------

// Event 是单个事件。字段与契约 §6.4 一致。
//
// 指针字段用于区分「未提供」与「提供零值」：
// matched_policy_id 未命中必须是 null，而不是空串。
type Event struct {
	EventID               string   `json:"event_id"`
	TS                    string   `json:"ts"`
	GatewayID             string   `json:"gateway_id,omitempty"`
	ClientIP              string   `json:"client_ip"`
	RealIPChain           []string `json:"real_ip_chain,omitempty"`
	Host                  string   `json:"host"`
	Path                  string   `json:"path"`
	Query                 string   `json:"query,omitempty"`
	Method                string   `json:"method"`
	Scheme                string   `json:"scheme,omitempty"`
	UA                    string   `json:"ua,omitempty"`
	Referer               string   `json:"referer,omitempty"`
	Status                int      `json:"status"`
	UpstreamStatus        *int     `json:"upstream_status"`
	DurationMS            int      `json:"duration_ms"`
	Bytes                 int64    `json:"bytes,omitempty"`
	MatchedPolicyID       *string  `json:"matched_policy_id"`
	MatchedPolicyPriority *int     `json:"matched_policy_priority,omitempty"`
	ConfigRevision        int64    `json:"config_revision"`
	Action                string   `json:"action"`
	ActionResult          string   `json:"action_result"`
	DecisionSource        string   `json:"decision_source"`
	DecisionLatencyUS     int      `json:"decision_latency_us,omitempty"`
	BlockTemplateID       *string  `json:"block_template_id,omitempty"`
	BlockTemplateVersion  *int     `json:"block_template_version,omitempty"`
	TraceID               string   `json:"trace_id,omitempty"`
	ClockSkewMS           *int     `json:"clock_skew_ms,omitempty"`
}

// nowRFC3339 返回毫秒精度的 RFC3339 时间。
func nowRFC3339() string {
	return time.Now().Format(time.RFC3339Nano)
}
