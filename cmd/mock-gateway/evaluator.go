package main

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
)

// evalRequest 是被求值的「模拟请求」。
type evalRequest struct {
	IP      string
	Host    string
	Path    string
	Method  string
	UA      string
	Headers map[string]string
	Query   map[string]string
	Cookies map[string]string
}

// evaluatePolicies 按数组顺序求值，返回第一条命中的策略（first-match 短路）。
//
// 注意：契约规定顺序由服务端排好（priority DESC, id ASC），
// 客户端**不得重新排序**，否则两台网关行为会不一致。
func evaluatePolicies(policies []Policy, req *evalRequest) (*Policy, error) {
	for i := range policies {
		ok, err := evalExpr(policies[i].Match, req)
		if err != nil {
			return nil, fmt.Errorf("策略 %s 求值失败：%w", policies[i].ID, err)
		}
		if ok {
			return &policies[i], nil
		}
	}
	return nil, nil
}

func evalExpr(raw json.RawMessage, req *evalRequest) (bool, error) {
	if len(raw) == 0 {
		return false, fmt.Errorf("match 为空")
	}

	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return false, err
	}

	if v, ok := node["all"]; ok {
		var list []json.RawMessage
		if err := json.Unmarshal(v, &list); err != nil {
			return false, err
		}
		for _, e := range list {
			ok, err := evalExpr(e, req)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	}

	if v, ok := node["any"]; ok {
		var list []json.RawMessage
		if err := json.Unmarshal(v, &list); err != nil {
			return false, err
		}
		for _, e := range list {
			ok, err := evalExpr(e, req)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}

	if v, ok := node["not"]; ok {
		ok, err := evalExpr(v, req)
		return !ok, err
	}

	return evalCondition(raw, req)
}

type condition struct {
	Field string          `json:"field"`
	Name  string          `json:"name"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

func evalCondition(raw json.RawMessage, req *evalRequest) (bool, error) {
	var c condition
	if err := json.Unmarshal(raw, &c); err != nil {
		return false, err
	}

	actual, ok := fieldValue(c.Field, c.Name, req)
	if !ok {
		return false, fmt.Errorf("不支持的字段 %q", c.Field)
	}

	single, list, err := conditionValues(c.Value)
	if err != nil {
		return false, err
	}

	ci := isCaseInsensitive(c.Field)

	switch c.Op {
	case "eq":
		return strEq(actual, single, ci), nil
	case "prefix":
		return strHasPrefix(actual, single, ci), nil
	case "suffix":
		return strHasSuffix(actual, single, ci), nil
	case "contains":
		return strContains(actual, single, ci), nil
	case "in":
		for _, v := range list {
			if strEq(actual, v, ci) {
				return true, nil
			}
		}
		return false, nil
	case "regex":
		// 大小写不敏感的字段（host/method/ua）用 (?i) 编译
		var re *regexp.Regexp
		var err error
		if ci {
			re, err = compileCachedCI(single)
		} else {
			re, err = compileCached(single)
		}
		if err != nil {
			return false, err
		}
		return re.MatchString(actual), nil
	case "in_cidr", "not_in_cidr":
		matched, err := matchAnyCIDR(actual, list)
		if err != nil {
			return false, err
		}
		if c.Op == "not_in_cidr" {
			return !matched, nil
		}
		return matched, nil
	default:
		return false, fmt.Errorf("不支持的操作符 %q", c.Op)
	}
}

func fieldValue(field, name string, req *evalRequest) (string, bool) {
	switch field {
	case "ip":
		return req.IP, true
	case "host":
		return req.Host, true
	case "path":
		return req.Path, true
	case "method":
		return req.Method, true
	case "ua":
		return req.UA, true
	case "header":
		return headerValue(req.Headers, name), true
	case "query":
		return req.Query[name], true
	case "cookie":
		return req.Cookies[name], true
	default:
		return "", false
	}
}

// headerValue 允许忽略大小写地查 header 名。
func headerValue(h map[string]string, name string) string {
	if v, ok := h[name]; ok {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func isCaseInsensitive(field string) bool {
	switch field {
	case "host", "method", "ua":
		return true
	}
	return false
}

// conditionValues 兼容单值与数组两种写法。
func conditionValues(raw json.RawMessage) (single string, list []string, err error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil, nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return "", nil, fmt.Errorf("value 既不是字符串也不是字符串数组：%s", string(raw))
	}
	if len(arr) == 0 {
		return "", nil, fmt.Errorf("value 数组不能为空")
	}
	return arr[0], arr, nil
}

func strEq(actual, want string, ci bool) bool {
	if ci {
		return strings.EqualFold(actual, want)
	}
	return actual == want
}

func strHasPrefix(actual, want string, ci bool) bool {
	if ci {
		return strings.HasPrefix(strings.ToLower(actual), strings.ToLower(want))
	}
	return strings.HasPrefix(actual, want)
}

func strHasSuffix(actual, want string, ci bool) bool {
	if ci {
		return strings.HasSuffix(strings.ToLower(actual), strings.ToLower(want))
	}
	return strings.HasSuffix(actual, want)
}

func strContains(actual, want string, ci bool) bool {
	if ci {
		return strings.Contains(strings.ToLower(actual), strings.ToLower(want))
	}
	return strings.Contains(actual, want)
}

var (
	regexCache   sync.Map // pattern -> *regexp.Regexp
	regexCICache sync.Map
	cidrCache    sync.Map // value -> []*net.IPNet
)

func compileCached(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则非法 %q：%w", pattern, err)
	}
	regexCache.Store(pattern, re)
	return re, nil
}

func compileCachedCI(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCICache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("正则非法 %q：%w", pattern, err)
	}
	regexCICache.Store(pattern, re)
	return re, nil
}

func matchAnyCIDR(ip string, cidrs []string) (bool, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false, nil // 非法 IP 一律不命中，不报错
	}
	for _, c := range cidrs {
		nets, err := cidrCached(c)
		if err != nil {
			return false, err
		}
		for _, n := range nets {
			if n.Contains(parsed) {
				return true, nil
			}
		}
	}
	return false, nil
}

// cidrCached 同时支持单地址与 CIDR。
func cidrCached(value string) ([]*net.IPNet, error) {
	if v, ok := cidrCache.Load(value); ok {
		return v.([]*net.IPNet), nil
	}
	if _, ipnet, err := net.ParseCIDR(value); err == nil {
		cidrCache.Store(value, []*net.IPNet{ipnet})
		return []*net.IPNet{ipnet}, nil
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, fmt.Errorf("非法 IP/CIDR %q", value)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	ipnet := &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
	cidrCache.Store(value, []*net.IPNet{ipnet})
	return []*net.IPNet{ipnet}, nil
}
