package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// logger 是 mock 内部的简单日志函数。
type logger func(format string, args ...any)

// snapshot 是本地策略快照。
type snapshot struct {
	Revision  int64     `json:"revision"`
	FetchedAt time.Time `json:"fetched_at"`
	Server    string    `json:"server"`
	Policies  []Policy  `json:"policies"`
}

// policyState 用读写锁管理快照：整体替换，绝不增量合并。
type policyState struct {
	mu   sync.RWMutex
	snap snapshot
}

func (s *policyState) get() snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func (s *policyState) apply(sn snapshot) {
	s.mu.Lock()
	s.snap = sn
	s.mu.Unlock()
}

// loadSnapshot 从磁盘恢复快照，返回可交给 poller 的初始 revision。
func loadSnapshot(cfg *Config, state *policyState, log logger) int64 {
	path := cfg.Poll.PersistPath
	if path == "" {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0 // 首次启动没有快照是正常的
	}
	var sn snapshot
	if err := json.Unmarshal(raw, &sn); err != nil {
		log("警告：磁盘快照解析失败，忽略：%v", err)
		return 0
	}
	if sn.Server != cfg.ShieldX.BaseURL {
		log("警告：磁盘快照来自 %s，与当前 %s 不一致，忽略", sn.Server, cfg.ShieldX.BaseURL)
		return 0
	}
	state.apply(sn)
	log("已从磁盘恢复快照：revision=%d policies=%d", sn.Revision, len(sn.Policies))
	return sn.Revision
}

// saveSnapshot 原子落盘当前快照；失败只记日志，不影响运行。
func saveSnapshot(cfg *Config, sn snapshot, log logger) {
	path := cfg.Poll.PersistPath
	if path == "" {
		return
	}
	raw, err := json.MarshalIndent(sn, "", "  ")
	if err != nil {
		log("警告：序列化快照失败：%v", err)
		return
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log("警告：创建快照目录失败：%v", err)
			return
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		log("警告：写入快照失败：%v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log("警告：落盘快照失败：%v", err)
	}
}
