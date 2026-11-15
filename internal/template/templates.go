package template

import (
	"fmt"
	"sync"
	"time"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/contract"
)

// Cache 缓存拦截模板，按 ID 保存当前版本。
type Cache struct {
	mu sync.RWMutex
	m  map[string]contract.Template
}

func NewCache() *Cache {
	return &Cache{m: map[string]contract.Template{}}
}

func (c *Cache) Get(id string) (contract.Template, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.m[id]
	return t, ok
}

func (c *Cache) Replace(list []contract.Template) {
	m := make(map[string]contract.Template, len(list))
	for _, t := range list {
		m[t.ID] = t
	}
	c.mu.Lock()
	c.m = m
	c.mu.Unlock()
}

func (c *Cache) Snapshot() []contract.Template {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]contract.Template, 0, len(c.m))
	for _, t := range c.m {
		out = append(out, t)
	}
	return out
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}

// MissingTemplates 返回策略里引用到、但本地没有的模板 ID。
func MissingTemplates(policies []contract.Policy, cache *Cache) []string {
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
		if _, ok := cache.Get(id); ok {
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

// RenderForPolicy 渲染拦截页；任何异常都回退内置页并返回 false。
func RenderForPolicy(cache *Cache, p *contract.Policy, vars map[string]string) (string, contract.Template, bool) {
	if p != nil {
		if id := p.Action.ParamString("template_id"); id != "" {
			if t, ok := cache.Get(id); ok {
				merged := map[string]string{}
				for k, v := range t.Vars {
					merged[k] = v
				}
				for k, v := range vars {
					merged[k] = v
				}
				out, err := Render(t.Content, merged)
				if err == nil {
					return out, t, true
				}
			}
		}
	}
	return renderBuiltin(vars["request_id"]), contract.Template{}, false
}

// RequestID 生成事件/请求标识：时间戳 + 序号，便于排查。
func RequestID(gatewayID string, seq int64) string {
	return fmt.Sprintf("%s-%d-%d", gatewayID, time.Now().UnixMilli(), seq)
}
