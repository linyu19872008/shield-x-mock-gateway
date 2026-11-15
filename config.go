package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration 让 YAML 里可以写 "5s" / "2m" 这样的时长。
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("非法时长 %q：%w", s, err)
	}
	*d = Duration(v)
	return nil
}

// D 返回标准 time.Duration。
func (d Duration) D() time.Duration { return time.Duration(d) }

// Config 是 mock-gateway 的完整配置。
type Config struct {
	ShieldX  ShieldXConfig  `yaml:"shieldx"`
	Poll     PollConfig     `yaml:"poll"`
	Template TemplateConfig `yaml:"template"`
	Report   ReportConfig   `yaml:"report"`
	Generate GenerateConfig `yaml:"generate"`
	Fault    FaultConfig    `yaml:"fault"`
	Ingress  IngressConfig  `yaml:"ingress"`
}

// ShieldXConfig 描述如何连接 shield-x。
type ShieldXConfig struct {
	BaseURL      string       `yaml:"base_url"`
	Token        string       `yaml:"token"`
	GatewayID    string       `yaml:"gateway_id"`
	AgentVersion string       `yaml:"agent_version"`
	Capabilities Capabilities `yaml:"capabilities"`
}

// Capabilities 是能力声明，序列化后放进 X-Shieldx-Capabilities。
type Capabilities struct {
	Actions             []string `yaml:"actions" json:"actions"`
	MatchFields         []string `yaml:"match_fields" json:"match_fields"`
	BlockTemplates      bool     `yaml:"block_templates" json:"block_templates"`
	TemplateSpecVersion int      `yaml:"template_spec_version" json:"template_spec_version"`
	MaxRules            int      `yaml:"max_rules" json:"max_rules"`
}

// JSON 返回紧凑 JSON（作为请求头上报）。
func (c Capabilities) JSON() string {
	b, err := json.Marshal(c)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// PollConfig 控制拉策略行为。
type PollConfig struct {
	Interval    Duration `yaml:"interval"`
	Timeout     Duration `yaml:"timeout"`
	PersistPath string   `yaml:"persist_path"`
	UseETag     bool     `yaml:"use_etag"`
}

// TemplateConfig 控制拉模板行为。
type TemplateConfig struct {
	FetchOnMissing bool     `yaml:"fetch_on_missing"`
	Timeout        Duration `yaml:"timeout"`
}

// ReportConfig 控制事件上报。
type ReportConfig struct {
	Interval   Duration `yaml:"interval"`
	BatchSize  int      `yaml:"batch_size"`
	Format     string   `yaml:"format"` // json | ndjson
	Gzip       bool     `yaml:"gzip"`
	Timeout    Duration `yaml:"timeout"`
	SpoolDir   string   `yaml:"spool_dir"`
	SpoolMaxMB int      `yaml:"spool_max_mb"`
	MaxBackoff Duration `yaml:"max_backoff"`
	Jitter     float64  `yaml:"jitter"`
}

// GenerateConfig 控制事件生成。
type GenerateConfig struct {
	Rate     int      `yaml:"rate"`
	Workers  int      `yaml:"workers"`
	Duration Duration `yaml:"duration"`
	// Backfill 非零时进入"回填模式"：把 rate×backfill 条事件的时间戳
	// 按 ramp 速率曲线铺满 [now-backfill, now]，并尽快发完，不等真实时间流逝。
	Backfill   Duration       `yaml:"backfill"`
	Ramp       []RampStep     `yaml:"ramp"`
	Paths      []PathWeight   `yaml:"paths"`
	HostPool   []HostWeight   `yaml:"host_pool"`
	UAPool     []UAWeight     `yaml:"ua_pool"`
	IPPools    IPPools        `yaml:"ip_pools"`
	StatusDist map[string]int `yaml:"status_distribution"`

	BadEventRatio     float64 `yaml:"bad_event_ratio"`
	GatewayErrorRatio float64 `yaml:"gateway_error_ratio"`
	TimeoutRatio      float64 `yaml:"timeout_ratio"`
	SampledRatio      float64 `yaml:"sampled_ratio"`
}

// RampStep 是速率爬坡台阶。
type RampStep struct {
	At   Duration `yaml:"at"`
	Rate int      `yaml:"rate"`
}

// PathWeight 描述一个被模拟的接口。
type PathWeight struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
	Weight int    `yaml:"weight"`
}

// HostWeight 描述域名权重。
type HostWeight struct {
	Host   string `yaml:"host"`
	Weight int    `yaml:"weight"`
}

// UAWeight 描述 User-Agent 权重。
type UAWeight struct {
	UA     string `yaml:"ua"`
	Weight int    `yaml:"weight"`
}

// IPPools 是 IP 段池。
type IPPools struct {
	Normal    []string `yaml:"normal"`
	Malicious []string `yaml:"malicious"`
	Internal  []string `yaml:"internal"`
}

// FaultConfig 用于故障注入，验证客户端的容错路径。
type FaultConfig struct {
	ShieldXDown        bool     `yaml:"shieldx_down"`
	HTTPStatusOverride int      `yaml:"http_status_override"`
	Latency            Duration `yaml:"latency"`
	DropPercent        int      `yaml:"drop_percent"`
}

// IngressConfig 控制新增的 HTTP 数据面（gateway 子命令）。
type IngressConfig struct {
	Listen          string   `yaml:"listen"`
	AllowRemote     bool     `yaml:"allow_remote"`
	MockStatus      int      `yaml:"mock_status"`
	MockContentType string   `yaml:"mock_content_type"`
	MockBody        string   `yaml:"mock_body"`
	ReadTimeout     Duration `yaml:"read_timeout"`
	WriteTimeout    Duration `yaml:"write_timeout"`
	TrustProxy      bool     `yaml:"trust_proxy"`
}

// defaultConfig 返回内置默认值；YAML 只覆盖出现的字段。
func defaultConfig() *Config {
	return &Config{
		ShieldX: ShieldXConfig{
			BaseURL:      "http://127.0.0.1:8080/shieldx",
			GatewayID:    "gw-mock-01",
			AgentVersion: "mock-gateway/" + version,
			Capabilities: Capabilities{
				Actions:             []string{"observe", "block", "redirect", "javascript_challenge"},
				MatchFields:         []string{"ip", "host", "path", "method", "header", "ua", "query", "cookie"},
				BlockTemplates:      true,
				TemplateSpecVersion: 1,
				MaxRules:            5000,
			},
		},
		Poll: PollConfig{
			Interval:    Duration(5 * time.Second),
			Timeout:     Duration(3 * time.Second),
			PersistPath: "./data/policy-snapshot.json",
			UseETag:     true,
		},
		Template: TemplateConfig{
			FetchOnMissing: true,
			Timeout:        Duration(3 * time.Second),
		},
		Report: ReportConfig{
			Interval:   Duration(2 * time.Second),
			BatchSize:  300,
			Format:     "json",
			Gzip:       true,
			Timeout:    Duration(10 * time.Second),
			SpoolDir:   "./data/spool",
			SpoolMaxMB: 200,
			MaxBackoff: Duration(60 * time.Second),
			Jitter:     0.2,
		},
		Generate: GenerateConfig{
			Rate:     200,
			Workers:  4,
			Duration: 0,
			Paths: []PathWeight{
				{Path: "/api/login", Method: "POST", Weight: 30},
				{Path: "/api/user/profile", Method: "GET", Weight: 50},
				{Path: "/api/order", Method: "POST", Weight: 10},
				{Path: "/admin", Method: "GET", Weight: 5},
				{Path: "/.env", Method: "GET", Weight: 5},
			},
			HostPool: []HostWeight{
				{Host: "api.example.com", Weight: 90},
				{Host: "admin.example.com", Weight: 10},
			},
			UAPool: []UAWeight{
				{UA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", Weight: 60},
				{UA: "curl/8.4.0", Weight: 20},
				{UA: "sqlmap/1.7.2#stable", Weight: 5},
				{UA: "Googlebot/2.1 (+http://www.google.com/bot.html)", Weight: 10},
				{UA: "python-requests/2.31.0", Weight: 5},
			},
			IPPools: IPPools{
				Normal:    []string{"198.51.100.0/24", "203.0.113.0/24"},
				Malicious: []string{"203.0.113.66"},
				Internal:  []string{"10.0.0.0/8"},
			},
			StatusDist: map[string]int{
				"200": 85,
				"302": 5,
				"404": 8,
				"500": 2,
			},
			BadEventRatio:     0.001,
			GatewayErrorRatio: 0.0005,
			TimeoutRatio:      0.001,
			SampledRatio:      0,
		},
		Ingress: IngressConfig{
			Listen:          "127.0.0.1:18081",
			MockStatus:      200,
			MockContentType: "text/plain; charset=utf-8",
			MockBody:        "mock upstream\n",
			ReadTimeout:     Duration(5 * time.Second),
			WriteTimeout:    Duration(10 * time.Second),
		},
	}
}

// LoadConfig 读取 YAML 配置；空路径表示只用默认值。
func LoadConfig(path string) (*Config, error) {
	cfg := defaultConfig()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取配置失败：%w", err)
		}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("解析配置失败：%w", err)
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.ShieldX.BaseURL == "" {
		return fmt.Errorf("配置缺少 shieldx.base_url")
	}
	if c.Generate.Rate < 0 {
		return fmt.Errorf("generate.rate 不能为负")
	}
	if c.Generate.Workers < 1 {
		c.Generate.Workers = 1
	}
	if c.Report.BatchSize < 1 {
		c.Report.BatchSize = 100
	}
	if c.Report.BatchSize > 1000 {
		c.Report.BatchSize = 1000
	}
	switch c.Report.Format {
	case "", "json":
		c.Report.Format = "json"
	case "ndjson":
	default:
		return fmt.Errorf("report.format 只支持 json | ndjson，当前 %q", c.Report.Format)
	}
	if c.Poll.Interval < Duration(time.Second) {
		c.Poll.Interval = Duration(time.Second)
	}
	if c.Report.Interval <= 0 {
		c.Report.Interval = Duration(2 * time.Second)
	}

	// 数据面默认值：YAML 里未出现的字段保持默认。
	if c.Ingress.Listen == "" {
		c.Ingress.Listen = "127.0.0.1:18081"
	}
	if c.Ingress.MockStatus == 0 {
		c.Ingress.MockStatus = 200
	}
	if c.Ingress.MockStatus < 100 || c.Ingress.MockStatus > 599 {
		return fmt.Errorf("ingress.mock_status 必须在 100~599 之间")
	}
	if c.Ingress.MockContentType == "" {
		c.Ingress.MockContentType = "text/plain; charset=utf-8"
	}
	if c.Ingress.MockBody == "" {
		c.Ingress.MockBody = "mock upstream\n"
	}
	if c.Ingress.ReadTimeout <= 0 {
		c.Ingress.ReadTimeout = Duration(5 * time.Second)
	}
	if c.Ingress.WriteTimeout <= 0 {
		c.Ingress.WriteTimeout = Duration(10 * time.Second)
	}
	return nil
}
