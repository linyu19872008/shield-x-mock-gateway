package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/linyu19872008/shield-x-client-go/agent"
)

func TestToPolicy(t *testing.T) {
	p := agent.Policy{
		ID:       "pol_1",
		Version:  2,
		Name:     "block bad ip",
		Priority: 1000,
		Match: agent.Match{All: []agent.Condition{{
			Field: "ip",
			Op:    "in_cidr",
			Value: agent.StringsValue([]string{"203.0.113.0/24"}),
		}}},
		Action: agent.Action{
			Type: agent.ActionBlock,
			Params: &agent.ActionParams{
				TemplateID: "tpl_1",
				StatusCode: 403,
			},
		},
	}

	out := toPolicy(p)
	if out.ID != p.ID || out.Priority != p.Priority {
		t.Fatalf("基础字段不符: %+v", out)
	}
	if out.Action.Type != "block" {
		t.Fatalf("action.type = %q", out.Action.Type)
	}
	if got := out.Action.ParamString("template_id"); got != "tpl_1" {
		t.Fatalf("template_id = %q", got)
	}
	if got := out.Action.ParamInt("status_code", 0); got != 403 {
		t.Fatalf("status_code = %d", got)
	}

	var match map[string]any
	if err := json.Unmarshal(out.Match, &match); err != nil {
		t.Fatalf("match 不是合法 JSON: %v", err)
	}
	if _, ok := match["all"]; !ok {
		t.Fatalf("match 缺少 all: %s", out.Match)
	}
}

func TestToAgentEvent(t *testing.T) {
	pid := "pol_1"
	ts := time.Now().UTC().Truncate(time.Second)
	e := Event{
		EventID:         "e-1",
		TS:              ts.Format(time.RFC3339Nano),
		ClientIP:        "203.0.113.7",
		Host:            "example.com",
		Path:            "/login",
		Method:          "GET",
		Status:          403,
		DurationMS:      3,
		ConfigRevision:  39,
		MatchedPolicyID: &pid,
		Action:          "block",
		ActionResult:    "executed",
		DecisionSource:  "policy",
	}

	out := toAgentEvent(e)
	if out.EventID != e.EventID || !out.TS.Equal(ts) {
		t.Fatalf("事件基础字段不符: %+v", out)
	}
	if out.Action != agent.ActionBlock || out.ActionResult != agent.ResultExecuted || out.DecisionSource != agent.DecisionPolicy {
		t.Fatalf("枚举转换不符: %+v", out)
	}
	if out.MatchedPolicyID == nil || *out.MatchedPolicyID != pid {
		t.Fatalf("matched_policy_id 转换不符: %v", out.MatchedPolicyID)
	}
}

func TestToAgentCapabilities(t *testing.T) {
	c := Capabilities{
		Actions:             []string{"observe", "block"},
		MatchFields:         []string{"ip", "host"},
		BlockTemplates:      true,
		TemplateSpecVersion: 1,
		MaxRules:            5000,
	}
	out := toAgentCapabilities(c)
	if len(out.Actions) != 2 || out.Actions[1] != agent.ActionBlock {
		t.Fatalf("actions 不符: %+v", out.Actions)
	}
	if !out.BlockTemplates || out.MaxRules != 5000 {
		t.Fatalf("能力转换不符: %+v", out)
	}
}
