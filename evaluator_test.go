package main

import (
	"encoding/json"
	"testing"
)

func mustPolicy(t *testing.T, raw string) Policy {
	t.Helper()
	var p Policy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("解析策略失败：%v\n%s", err, raw)
	}
	return p
}

func baseReq() *evalRequest {
	return &evalRequest{
		IP:     "203.0.113.66",
		Host:   "api.example.com",
		Path:   "/api/login",
		Method: "POST",
		UA:     "sqlmap/1.7.2#stable",
		Headers: map[string]string{
			"X-Forwarded-For": "203.0.113.66",
			"User-Agent":      "sqlmap/1.7.2#stable",
		},
		Query:   map[string]string{"debug": "1"},
		Cookies: map[string]string{"session": "abc"},
	}
}

func TestEvaluatorExamples(t *testing.T) {
	cases := []struct {
		name    string
		policy  string
		req     func(*evalRequest)
		wantHit bool
	}{
		{
			name:    "IP 段命中",
			policy:  `{"id":"p1","match":{"all":[{"field":"ip","op":"in_cidr","value":["203.0.113.0/24"]}]},"action":{"type":"block"}}`,
			wantHit: true,
		},
		{
			name:    "IP 段不命中",
			policy:  `{"id":"p1","match":{"all":[{"field":"ip","op":"in_cidr","value":["10.0.0.0/8"]}]},"action":{"type":"block"}}`,
			wantHit: false,
		},
		{
			name:    "path 精确 + method",
			policy:  `{"id":"p2","match":{"all":[{"field":"path","op":"eq","value":"/api/login"},{"field":"method","op":"eq","value":"POST"}]},"action":{"type":"block"}}`,
			wantHit: true,
		},
		{
			name:    "method 不匹配则整体不命中",
			policy:  `{"id":"p2","match":{"all":[{"field":"path","op":"eq","value":"/api/login"},{"field":"method","op":"eq","value":"GET"}]},"action":{"type":"block"}}`,
			wantHit: false,
		},
		{
			name:    "any：UA 含 sqlmap",
			policy:  `{"id":"p3","match":{"any":[{"field":"ua","op":"contains","value":"sqlmap"},{"field":"ua","op":"contains","value":"nikto"}]},"action":{"type":"observe"}}`,
			wantHit: true,
		},
		{
			name:    "not：除内网外一律命中",
			policy:  `{"id":"p4","match":{"all":[{"field":"path","op":"prefix","value":"/admin"},{"not":{"field":"ip","op":"in_cidr","value":["10.0.0.0/8"]}}]},"action":{"type":"block"}}`,
			req:     func(r *evalRequest) { r.Path = "/admin/users" },
			wantHit: true,
		},
		{
			name:    "not：内网地址豁免",
			policy:  `{"id":"p4","match":{"all":[{"field":"path","op":"prefix","value":"/admin"},{"not":{"field":"ip","op":"in_cidr","value":["10.0.0.0/8"]}}]},"action":{"type":"block"}}`,
			req:     func(r *evalRequest) { r.Path = "/admin/users"; r.IP = "10.1.2.3" },
			wantHit: false,
		},
		{
			name:    "header 正则",
			policy:  `{"id":"p5","match":{"all":[{"field":"header","name":"X-Forwarded-For","op":"regex","value":"[^0-9., ]"}]},"action":{"type":"block"}}`,
			req:     func(r *evalRequest) { r.Headers["X-Forwarded-For"] = "1.2.3.4; DROP" },
			wantHit: true,
		},
		{
			name:    "query 精确",
			policy:  `{"id":"p6","match":{"all":[{"field":"query","name":"debug","op":"eq","value":"1"}]},"action":{"type":"observe"}}`,
			wantHit: true,
		},
		{
			name:    "cookie 前缀",
			policy:  `{"id":"p7","match":{"all":[{"field":"cookie","name":"session","op":"prefix","value":"ab"}]},"action":{"type":"observe"}}`,
			wantHit: true,
		},
		{
			name:    "host 大小写不敏感",
			policy:  `{"id":"p8","match":{"all":[{"field":"host","op":"eq","value":"API.EXAMPLE.COM"}]},"action":{"type":"observe"}}`,
			wantHit: true,
		},
		{
			name:    "path 大小写敏感",
			policy:  `{"id":"p9","match":{"all":[{"field":"path","op":"eq","value":"/API/LOGIN"}]},"action":{"type":"observe"}}`,
			wantHit: false,
		},
		{
			name:    "IPv6 CIDR",
			policy:  `{"id":"p10","match":{"all":[{"field":"ip","op":"in_cidr","value":["2001:db8::/32"]}]},"action":{"type":"block"}}`,
			req:     func(r *evalRequest) { r.IP = "2001:db8::1" },
			wantHit: true,
		},
		{
			name:    "非法 IP 不命中且不报错",
			policy:  `{"id":"p11","match":{"all":[{"field":"ip","op":"in_cidr","value":["203.0.113.0/24"]}]},"action":{"type":"block"}}`,
			req:     func(r *evalRequest) { r.IP = "not-an-ip" },
			wantHit: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := baseReq()
			if c.req != nil {
				c.req(req)
			}
			p := mustPolicy(t, c.policy)
			hit, err := evaluatePolicies([]Policy{p}, req)
			if err != nil {
				t.Fatalf("求值出错：%v", err)
			}
			if got := hit != nil; got != c.wantHit {
				t.Fatalf("命中 = %v，期望 %v", got, c.wantHit)
			}
		})
	}
}

func TestEvaluatorFirstMatchShortCircuit(t *testing.T) {
	policies := []Policy{
		mustPolicy(t, `{"id":"first","priority":1000,"match":{"all":[{"field":"ip","op":"in_cidr","value":["203.0.113.0/24"]}]},"action":{"type":"observe"}}`),
		mustPolicy(t, `{"id":"second","priority":500,"match":{"all":[{"field":"path","op":"prefix","value":"/api"}]},"action":{"type":"block"}}`),
	}
	hit, err := evaluatePolicies(policies, baseReq())
	if err != nil {
		t.Fatal(err)
	}
	if hit == nil || hit.ID != "first" {
		t.Fatalf("应命中第一条，实际 %+v", hit)
	}
}

func TestEvaluatorEmptyPolicies(t *testing.T) {
	hit, err := evaluatePolicies(nil, baseReq())
	if err != nil {
		t.Fatal(err)
	}
	if hit != nil {
		t.Fatalf("空策略集应返回 nil，实际 %+v", hit)
	}
}

func TestEvaluatorUnknownField(t *testing.T) {
	p := mustPolicy(t, `{"id":"bad","match":{"all":[{"field":"body","op":"contains","value":"x"}]},"action":{"type":"block"}}`)
	if _, err := evaluatePolicies([]Policy{p}, baseReq()); err == nil {
		t.Fatal("未知字段应报错，避免静默放行")
	}
}

func TestEvaluatorBadRegex(t *testing.T) {
	p := mustPolicy(t, `{"id":"bad","match":{"all":[{"field":"path","op":"regex","value":"["}]},"action":{"type":"block"}}`)
	if _, err := evaluatePolicies([]Policy{p}, baseReq()); err == nil {
		t.Fatal("非法正则应报错")
	}
}
