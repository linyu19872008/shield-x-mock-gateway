package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// generator 生成模拟请求、求值策略、产出事件。
type generator struct {
	cfg   *Config
	state *policyState
	tpl   *templateCache
	st    *stats
	emit  func(Event)
	seq   atomic.Int64

	// 回填模式：时间戳铺在 [start, start+window]，窗口在启动时固定
	backfillStart  time.Time
	backfillWindow time.Duration
	backfillCurve  []rampWindow

	wg sync.WaitGroup
}

// rampWindow 是回填时用于分布时间戳的一个时间段及其权重。
type rampWindow struct {
	offset time.Duration // 相对回填窗口起点的偏移
	width  time.Duration
	weight float64
}

// initBackfill 预计算时间戳分布曲线：没有 ramp 就均匀铺满，有 ramp 则按各档速率加权。
func (g *generator) initBackfill() {
	d := g.cfg.Generate.Backfill.D()
	if d <= 0 {
		return
	}
	g.backfillWindow = d
	g.backfillStart = time.Now().Add(-d)

	ramp := append([]RampStep(nil), g.cfg.Generate.Ramp...)
	if len(ramp) == 0 {
		g.backfillCurve = []rampWindow{{offset: 0, width: d, weight: float64(d.Seconds()) * float64(max(g.cfg.Generate.Rate, 1))}}
		return
	}
	for i, step := range ramp {
		end := d
		if i+1 < len(ramp) {
			end = ramp[i+1].At.D()
		}
		from := step.At.D()
		if from >= d {
			break
		}
		if end > d {
			end = d
		}
		if end <= from {
			continue
		}
		rate := float64(step.Rate)
		if rate <= 0 {
			rate = float64(max(g.cfg.Generate.Rate, 1))
		}
		g.backfillCurve = append(g.backfillCurve, rampWindow{
			offset: from,
			width:  end - from,
			weight: (end - from).Seconds() * rate,
		})
	}
	if len(g.backfillCurve) == 0 {
		g.backfillCurve = []rampWindow{{offset: 0, width: d, weight: float64(d.Seconds()) * float64(max(g.cfg.Generate.Rate, 1))}}
	}
}

// eventTS 返回事件时间戳：回填模式按曲线采样窗口内的时间，否则用当前时间。
func (g *generator) eventTS(rnd *mrand.Rand) string {
	if g.backfillWindow <= 0 || len(g.backfillCurve) == 0 {
		return nowRFC3339()
	}
	var total float64
	for _, w := range g.backfillCurve {
		total += w.weight
	}
	u := rnd.Float64() * total
	for _, w := range g.backfillCurve {
		if u <= w.weight {
			off := w.offset + time.Duration(rnd.Float64()*float64(w.width))
			return g.backfillStart.Add(off).UTC().Format(time.RFC3339Nano)
		}
		u -= w.weight
	}
	return nowRFC3339()
}

// wait 等待所有 worker 生成完毕（仅回填模式会自然结束）。
func (g *generator) wait() { g.wg.Wait() }

func (g *generator) nextSeq() int64 { return g.seq.Add(1) }

// run 启动 N 个 worker，从令牌通道领取配额后各生成一条事件。
func (g *generator) run(ctx context.Context, tokens <-chan struct{}, deadline time.Time) {
	workers := g.cfg.Generate.Workers
	for i := 0; i < workers; i++ {
		g.wg.Add(1)
		go func(workerID int) {
			defer g.wg.Done()
			rnd := mrand.New(mrand.NewSource(time.Now().UnixNano() + int64(workerID)*7919))
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-tokens:
					if !ok {
						return
					}
					if !deadline.IsZero() && time.Now().After(deadline) {
						return
					}
					g.emit(g.makeEvent(rnd))
				}
			}
		}(i)
	}
}

// pumpTokens 是全局令牌桶：每 10ms 补充一批令牌，支持 ramp 速率曲线。
func (g *generator) pumpTokens(ctx context.Context, out chan<- struct{}) {
	// 回填模式：总数 = rate × backfill 秒，尽快全部发出后关闭令牌通道
	if d := g.cfg.Generate.Backfill.D(); d > 0 {
		total := int64(g.cfg.Generate.Rate) * int64(d.Seconds())
		for i := int64(0); i < total; i++ {
			select {
			case out <- struct{}{}:
			case <-ctx.Done():
				close(out)
				return
			}
		}
		close(out)
		return
	}

	const tick = 10 * time.Millisecond
	start := time.Now()
	acc := 0.0

	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rate := currentRate(g.cfg.Generate, time.Since(start))
			acc += float64(rate) * tick.Seconds()
			n := int(acc)
			acc -= float64(n)
			for i := 0; i < n; i++ {
				select {
				case out <- struct{}{}:
				default:
					// 令牌通道满说明后端跟不上，直接丢弃令牌而不是阻塞，
					// 这样压测压力不会因为本地阻塞而失真。
				}
			}
		}
	}
}

// currentRate 计算当前目标速率：ramp 为空时恒为 Rate，否则取最后一个已到达的台阶。
func currentRate(cfg GenerateConfig, elapsed time.Duration) int {
	if len(cfg.Ramp) == 0 {
		return cfg.Rate
	}
	rate := 0
	for _, step := range cfg.Ramp {
		if elapsed >= step.At.D() {
			rate = step.Rate
		}
	}
	return rate
}

// makeEvent 构造一条完整事件：模拟请求 → 求值 → 决定响应与动作。
func (g *generator) makeEvent(rnd *mrand.Rand) Event {
	seq := g.nextSeq()
	path, method := pickPath(rnd, g.cfg.Generate.Paths)
	host := pickHost(rnd, g.cfg.Generate.HostPool)
	ua := pickUA(rnd, g.cfg.Generate.UAPool)
	ip := pickIP(rnd, g.cfg.Generate.IPPools)

	reqID := fmt.Sprintf("%s-%d-%d", g.cfg.ShieldX.GatewayID, time.Now().UnixMilli(), seq)

	req := &evalRequest{
		IP:     ip,
		Host:   host,
		Path:   path,
		Method: method,
		UA:     ua,
		Headers: map[string]string{
			"User-Agent":      ua,
			"X-Forwarded-For": ip,
			"Accept":          "*/*",
		},
		Query:   map[string]string{},
		Cookies: map[string]string{},
	}
	if rnd.Intn(10) == 0 {
		req.Query["debug"] = "1"
	}
	if rnd.Intn(5) == 0 {
		req.Cookies["session"] = randomHex(rnd, 8)
	}

	snap := g.state.get()
	policy, err := evaluatePolicies(snap.Policies, req)

	upstreamStatus := pickStatus(rnd, g.cfg.Generate.StatusDist)
	ev := Event{
		EventID:        fmt.Sprintf("%s-%d", g.cfg.ShieldX.GatewayID, seq),
		TS:             g.eventTS(rnd),
		GatewayID:      g.cfg.ShieldX.GatewayID,
		ClientIP:       ip,
		RealIPChain:    []string{ip},
		Host:           host,
		Path:           path,
		Method:         method,
		Scheme:         "http",
		UA:             truncate(ua, 256),
		Status:         upstreamStatus,
		UpstreamStatus: &upstreamStatus,
		DurationMS:     rnd.Intn(200) + 1,
		Bytes:          int64(rnd.Intn(5000) + 100),
		ConfigRevision: snap.Revision,
		Action:         "none",
		ActionResult:   "executed",
		DecisionSource: "none",
		TraceID:        randomHex(rnd, 8),
	}
	ev.DecisionLatencyUS = rnd.Intn(180) + 20

	g.applyDecision(rnd, &ev, policy, err, reqID)

	// 异常注入：这三种是真实网关一定会有、但最容易忘记造的事件
	if hit(rnd, g.cfg.Generate.GatewayErrorRatio) {
		ev.MatchedPolicyID = nil
		ev.MatchedPolicyPriority = nil
		ev.Action = "none"
		ev.ActionResult = "executed"
		ev.DecisionSource = "gateway_error"
	}

	if hit(rnd, g.cfg.Generate.TimeoutRatio) {
		ev.Status = 504
		ev.UpstreamStatus = nil
		ev.DurationMS = 30000
	}

	if hit(rnd, g.cfg.Generate.BadEventRatio) {
		// 故意构造非法事件，用于验证服务端 rejected 统计
		ev.ClientIP = "not-an-ip"
	}

	g.st.Generated.Add(1)
	return ev
}

// applyDecision 根据策略命中结果填充动作与响应。
func (g *generator) applyDecision(rnd *mrand.Rand, ev *Event, p *Policy, err error, reqID string) {
	if err != nil {
		// 策略引擎异常：必须上报 gateway_error，否则防护失效会被掩盖
		ev.DecisionSource = "gateway_error"
		return
	}
	if p == nil {
		ev.DecisionSource = "none"
		return
	}

	id := p.ID
	prio := p.Priority
	ev.MatchedPolicyID = &id
	ev.MatchedPolicyPriority = &prio
	ev.DecisionSource = "policy"
	ev.Action = p.Action.Type

	vars := map[string]string{
		"request_id":    reqID,
		"timestamp":     ev.TS,
		"client_ip":     ev.ClientIP,
		"host":          ev.Host,
		"path":          ev.Path,
		"site_name":     g.cfg.ShieldX.GatewayID,
		"contact_email": "security@example.com",
	}

	switch p.Action.Type {
	case "observe":
		ev.ActionResult = "executed"

	case "block":
		_, tpl, ok := renderForPolicy(g.tpl, p, vars)
		ev.ActionResult = "executed"
		if !ok {
			ev.ActionResult = "failed" // 模板缺失 → 用了内置兜底页
		}
		status := p.Action.ParamInt("status_code", 0)
		if status == 0 {
			status = tpl.StatusCode
		}
		if status == 0 {
			status = 403
		}
		ev.Status = status
		ev.UpstreamStatus = nil
		if tpl.ID != "" {
			tid := tpl.ID
			tv := tpl.Version
			ev.BlockTemplateID = &tid
			ev.BlockTemplateVersion = &tv
		}

	case "redirect":
		ev.Status = p.Action.ParamInt("status_code", 302)
		ev.UpstreamStatus = nil
		ev.ActionResult = "executed"

	case "javascript_challenge":
		ev.Status = 200
		ev.UpstreamStatus = nil
		ev.ActionResult = "challenged"

	default:
		ev.ActionResult = "failed"
	}
}

// ---------- 权重与随机工具 ----------

func pickPath(rnd *mrand.Rand, list []PathWeight) (string, string) {
	idx := weightedIndex(rnd, len(list), func(i int) int { return list[i].Weight })
	return list[idx].Path, list[idx].Method
}

func pickHost(rnd *mrand.Rand, list []HostWeight) string {
	if len(list) == 0 {
		return "api.example.com"
	}
	idx := weightedIndex(rnd, len(list), func(i int) int { return list[i].Weight })
	return list[idx].Host
}

func pickUA(rnd *mrand.Rand, list []UAWeight) string {
	if len(list) == 0 {
		return "curl/8.0"
	}
	idx := weightedIndex(rnd, len(list), func(i int) int { return list[i].Weight })
	return list[idx].UA
}

func weightedIndex(rnd *mrand.Rand, n int, weight func(int) int) int {
	if n == 0 {
		return 0
	}
	total := 0
	for i := 0; i < n; i++ {
		w := weight(i)
		if w > 0 {
			total += w
		}
	}
	if total == 0 {
		return rnd.Intn(n)
	}
	pick := rnd.Intn(total)
	acc := 0
	for i := 0; i < n; i++ {
		w := weight(i)
		if w <= 0 {
			continue
		}
		acc += w
		if pick < acc {
			return i
		}
	}
	return n - 1
}

// pickIP 从 IP 段池里随机取地址：70% 正常、20% 恶意、10% 内网。
func pickIP(rnd *mrand.Rand, pools IPPools) string {
	var pool []string
	switch r := rnd.Intn(100); {
	case r < 70 && len(pools.Normal) > 0:
		pool = pools.Normal
	case r < 90 && len(pools.Malicious) > 0:
		pool = pools.Malicious
	case len(pools.Internal) > 0:
		pool = pools.Internal
	default:
		pool = pools.Normal
	}
	if len(pool) == 0 {
		return "198.51.100.1"
	}
	return randomIPIn(rnd, pool[rnd.Intn(len(pool))])
}

// randomIPIn 在给定 CIDR 内生成随机 IP。
//
// 之前这里固定只随机末两段（忽略掩码），对 /24 会生成 203.0.229.x 这种
// 网络外的地址，导致"按 IP 段匹配"的策略命中率只有 1/256，回填/压测数据
// 看起来像策略不生效。现在按掩码随机主机位，并避开网络地址与广播地址。
func randomIPIn(rnd *mrand.Rand, cidr string) string {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		if single := net.ParseIP(cidr); single != nil {
			return single.String()
		}
		return "198.51.100.1"
	}

	ones, bits := ipnet.Mask.Size()
	if ones < 0 {
		return ipnet.IP.String()
	}

	base := ipnet.IP
	if bits == 32 {
		b4 := base.To4()
		if b4 == nil {
			return ipnet.IP.String()
		}
		mask := ipnet.Mask
		buf := make([]byte, 4)
		for i := range buf {
			buf[i] = byte(rnd.Intn(256))
		}
		out := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			out[i] = (buf[i] & ^mask[i]) | (b4[i] & mask[i])
		}
		if bits-ones >= 2 {
			for out[3] == 0 || out[3] == 255 { // 避开网络/广播地址
				out[3] = byte(rnd.Intn(256))
			}
		}
		return out.String()
	}

	// IPv6：同样按掩码随机主机位
	b16 := base.To16()
	if b16 == nil {
		return ipnet.IP.String()
	}
	buf := make([]byte, 16)
	for i := 0; i < 16; i++ {
		buf[i] = byte(rnd.Intn(256))
	}
	out := make(net.IP, 16)
	for i := 0; i < 16; i++ {
		out[i] = (buf[i] & ^ipnet.Mask[i]) | (b16[i] & ipnet.Mask[i])
	}
	return out.String()
}

func pickStatus(rnd *mrand.Rand, dist map[string]int) int {
	if len(dist) == 0 {
		return 200
	}
	type kv struct {
		code   int
		weight int
	}
	list := make([]kv, 0, len(dist))
	for k, w := range dist {
		c, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		list = append(list, kv{c, w})
	}
	idx := weightedIndex(rnd, len(list), func(i int) int { return list[i].weight })
	return list[idx].code
}

func hit(rnd *mrand.Rand, ratio float64) bool {
	if ratio <= 0 {
		return false
	}
	return rnd.Float64() < ratio
}

func randomHex(rnd *mrand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rnd.Intn(256))
	}
	return hex.EncodeToString(b)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// randomBytes 用于生成不可预测的随机串（保留给后续用途）。
func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
