package main

import (
	"math/rand"
	"net"
	"testing"
)

// randomIPIn 曾经忽略掩码，只随机末两段：对 /24 会生成网络外的地址，
// 导致按 IP 段匹配的策略命中率极低。这个测试锁住"必须落在 CIDR 内"。
func TestPickIPStaysInsideDeclaredCIDR(t *testing.T) {
	cfg, err := LoadConfig("../../config.backfill.yaml")
	if err != nil {
		t.Fatalf("加载示例配置失败：%v", err)
	}
	pools := append(append([]string{}, cfg.Generate.IPPools.Normal...),
		cfg.Generate.IPPools.Malicious...)
	pools = append(pools, cfg.Generate.IPPools.Internal...)

	nets := make([]*net.IPNet, 0, len(pools))
	for _, c := range pools {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("示例配置里的 CIDR 非法：%s", c)
		}
		nets = append(nets, n)
	}

	rnd := rand.New(rand.NewSource(7))
	for i := 0; i < 5000; i++ {
		ip := net.ParseIP(pickIP(rnd, cfg.Generate.IPPools))
		if ip == nil {
			t.Fatalf("生成了非法 IP")
		}
		ok := false
		for _, n := range nets {
			if n.Contains(ip) {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("IP %s 不在任何声明的 CIDR 内", ip)
		}
	}
}

func TestRandomIPInRespectsMask(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	_, n, _ := net.ParseCIDR("203.0.113.0/24")
	for i := 0; i < 200; i++ {
		ip := net.ParseIP(randomIPIn(rnd, "203.0.113.0/24"))
		if ip == nil || !n.Contains(ip) {
			t.Fatalf("/24 生成的 IP 越界：%v", ip)
		}
	}
	if got := randomIPIn(rnd, "10.1.2.3"); got != "10.1.2.3" {
		t.Fatalf("单个 IP 应原样返回，得到 %s", got)
	}
}
