// mock-gateway 是 shield-x 的开发数据源、契约一致性测试客户端与压测发生器。
//
// 求值、模板渲染、流量生成由 mock 独立实现，保留「契约可被独立实施」的验证能力；
// 与 shield-x 的所有 agent 交互（拉策略/模板、上报、spool 补发）统一经由
// shield-x-client-go 的 agentlink 适配层，使 mock 同时成为 SDK 的 E2E 驱动。
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "0.1.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "conformance":
		os.Exit(cmdConformance(os.Args[2:]))
	case "serve":
		os.Exit(cmdServe(os.Args[2:]))
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "loadtest":
		os.Exit(cmdLoadTest(os.Args[2:]))
	case "gateway":
		os.Exit(cmdGateway(os.Args[2:]))
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		fmt.Fprintf(os.Stderr, "\n未知子命令：%s\n", os.Args[1])
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mock-gateway — shield-x 的开发数据源 / 契约测试 / 压测发生器

用法：
  mock-gateway <子命令> [参数]

子命令：
  conformance   跑模板渲染一致性测试向量（不依赖 shield-x）
  serve         控制台：HTTP API + 可启停的运行控制
  gateway       数据面：接收真实请求，本地求值后返回 mock 响应
  run           常驻：轮询策略 + 持续生成并上报事件
  loadtest      压测：固定速率 + 时长，输出 JSON 报告
  version       打印版本

示例：
  mock-gateway conformance --vectors ./testdata/template-conformance/cases.json
  mock-gateway serve --config ./config.example.yaml
  mock-gateway gateway --config ./config.example.yaml
  mock-gateway run --config ./config.example.yaml
  mock-gateway loadtest --config ./config.yaml --rate 3000 --duration 10m
`)
}

func cmdConformance(args []string) int {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)
	vectors := fs.String("vectors", "./testdata/template-conformance/cases.json", "测试向量文件路径")
	verbose := fs.Bool("v", false, "输出每条用例的说明")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return runConformance(*vectors, *verbose)
}
