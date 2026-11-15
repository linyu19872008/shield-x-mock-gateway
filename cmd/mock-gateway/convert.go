package main

import (
	"encoding/json"
	"time"

	"github.com/linyu19872008/shield-x-client-go/agent"
)

// 本文件是 mock 自有模型与 client-go 模型的唯一转换点。

func toAgentCapabilities(c Capabilities) agent.Capabilities {
	actions := make([]agent.ActionType, 0, len(c.Actions))
	for _, a := range c.Actions {
		actions = append(actions, agent.ActionType(a))
	}
	return agent.Capabilities{
		Actions:             actions,
		MatchFields:         c.MatchFields,
		BlockTemplates:      c.BlockTemplates,
		TemplateSpecVersion: c.TemplateSpecVersion,
		MaxRules:            c.MaxRules,
	}
}

func toSnapshot(s *agent.PolicySnapshot, server string) snapshot {
	policies := make([]Policy, 0, len(s.Policies))
	for _, p := range s.Policies {
		policies = append(policies, toPolicy(p))
	}
	return snapshot{
		Revision:  s.Revision,
		FetchedAt: time.Now(),
		Server:    server,
		Policies:  policies,
	}
}

func toPolicy(p agent.Policy) Policy {
	match, _ := json.Marshal(p.Match)

	action := Action{Type: string(p.Action.Type)}
	if ap := p.Action.Params; ap != nil {
		params := map[string]any{}
		if ap.TemplateID != "" {
			params["template_id"] = ap.TemplateID
		}
		if ap.StatusCode != 0 {
			params["status_code"] = ap.StatusCode
		}
		if ap.RedirectURL != "" {
			params["redirect_url"] = ap.RedirectURL
		}
		if ap.ChallengeTTLS != 0 {
			params["challenge_ttl_s"] = ap.ChallengeTTLS
		}
		action.Params = params
	}

	return Policy{
		ID:       p.ID,
		Version:  p.Version,
		Name:     p.Name,
		Priority: p.Priority,
		Match:    match,
		Action:   action,
	}
}

func toTemplates(s *agent.TemplateSnapshot) []Template {
	out := make([]Template, 0, len(s.Templates))
	for _, t := range s.Templates {
		out = append(out, Template{
			ID:          t.ID,
			Name:        t.Name,
			Version:     t.Version,
			Checksum:    t.Checksum,
			ContentType: t.ContentType,
			StatusCode:  t.StatusCode,
			Headers:     t.Headers,
			Content:     t.Content,
			Vars:        t.Vars,
		})
	}
	return out
}

func toAgentEvent(e Event) agent.Event {
	ts, err := time.Parse(time.RFC3339Nano, e.TS)
	if err != nil {
		ts = time.Now()
	}
	return agent.Event{
		EventID:               e.EventID,
		TS:                    ts,
		GatewayID:             e.GatewayID,
		ClientIP:              e.ClientIP,
		RealIPChain:           e.RealIPChain,
		Host:                  e.Host,
		Path:                  e.Path,
		Query:                 e.Query,
		Method:                e.Method,
		Scheme:                e.Scheme,
		UA:                    e.UA,
		Referer:               e.Referer,
		Status:                e.Status,
		UpstreamStatus:        e.UpstreamStatus,
		DurationMS:            e.DurationMS,
		Bytes:                 e.Bytes,
		MatchedPolicyID:       e.MatchedPolicyID,
		MatchedPolicyPriority: e.MatchedPolicyPriority,
		ConfigRevision:        e.ConfigRevision,
		Action:                agent.ActionType(e.Action),
		ActionResult:          agent.ActionResult(e.ActionResult),
		DecisionSource:        agent.DecisionSource(e.DecisionSource),
		DecisionLatencyUS:     e.DecisionLatencyUS,
		BlockTemplateID:       e.BlockTemplateID,
		BlockTemplateVersion:  e.BlockTemplateVersion,
		TraceID:               e.TraceID,
		ClockSkewMS:           e.ClockSkewMS,
	}
}
