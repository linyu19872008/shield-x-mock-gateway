package main

import (
	"fmt"
	"sync"
	"time"
)

// templateCache 缓存拦截模板，按 ID 保存当前版本。
type templateCache struct {
	mu sync.RWMutex
	m  map[string]Template
}

func newTemplateCache() *templateCache {
	return &templateCache{m: map[string]Template{}}
}

func (c *templateCache) get(id string) (Template, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.m[id]
	return t, ok
}

func (c *templateCache) replace(list []Template) {
	m := make(map[string]Template, len(list))
	for _, t := range list {
		m[t.ID] = t
	}
	c.mu.Lock()
	c.m = m
	c.mu.Unlock()
}

func (c *templateCache) snapshot() []Template {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Template, 0, len(c.m))
	for _, t := range c.m {
		out = append(out, t)
	}
	return out
}

func (c *templateCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}

// missingTemplates 返回策略里引用到、但本地没有的模板 ID。
func missingTemplates(policies []Policy, cache *templateCache) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range policies {
		if p.Action.Type != "block" {
			continue
		}
		id := p.Action.ParamString("template_id")
		if id == "" {
			continue
		}
		if _, ok := cache.get(id); ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// renderForPolicy 渲染拦截页；任何异常都回退内置页并返回 false。
func renderForPolicy(cache *templateCache, p *Policy, vars map[string]string) (string, Template, bool) {
	if p != nil {
		if id := p.Action.ParamString("template_id"); id != "" {
			if t, ok := cache.get(id); ok {
				merged := map[string]string{}
				for k, v := range t.Vars {
					merged[k] = v
				}
				for k, v := range vars {
					merged[k] = v
				}
				out, err := renderTemplate(t.Content, merged)
				if err == nil {
					return out, t, true
				}
			}
		}
	}
	return renderBuiltin(vars["request_id"]), Template{}, false
}

// requestID 生成事件/请求标识：时间戳 + 序号，便于排查。
func requestID(gatewayID string, seq int64) string {
	return fmt.Sprintf("%s-%d-%d", gatewayID, time.Now().UnixMilli(), seq)
}
