package state

import (
	"github.com/linyu19872008/shield-x-mock-gateway/internal/config"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/contract"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/logging"
)

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State manages the complete policy snapshot with an atomic replace.
type State struct {
	mu   sync.RWMutex
	snap contract.Snapshot
}

// Get returns the current snapshot.
func (s *State) Get() contract.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Apply replaces the current snapshot atomically.
func (s *State) Apply(sn contract.Snapshot) {
	s.mu.Lock()
	s.snap = sn
	s.mu.Unlock()
}

// loadSnapshot 从磁盘恢复快照，返回可交给 poller 的初始 revision。
func Load(cfg *config.Config, state *State, log logging.Logger) int64 {
	path := cfg.Poll.PersistPath
	if path == "" {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0 // 首次启动没有快照是正常的
	}
	var sn contract.Snapshot
	if err := json.Unmarshal(raw, &sn); err != nil {
		log("警告：磁盘快照解析失败，忽略：%v", err)
		return 0
	}
	if sn.Server != cfg.ShieldX.BaseURL {
		log("警告：磁盘快照来自 %s，与当前 %s 不一致，忽略", sn.Server, cfg.ShieldX.BaseURL)
		return 0
	}
	state.Apply(sn)
	log("已从磁盘恢复快照：revision=%d policies=%d", sn.Revision, len(sn.Policies))
	return sn.Revision
}

// saveSnapshot 原子落盘当前快照；失败只记日志，不影响运行。
func Save(cfg *config.Config, sn contract.Snapshot, log logging.Logger) {
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
