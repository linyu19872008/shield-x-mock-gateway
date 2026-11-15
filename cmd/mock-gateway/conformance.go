package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// conformanceCase 对应 testdata/template-conformance/cases.json 的一条向量。
type conformanceCase struct {
	ID             string            `json:"id"`
	Description    string            `json:"description"`
	Template       string            `json:"template"`
	TemplateRepeat *repeatSpec       `json:"template_repeat"`
	Vars           map[string]string `json:"vars"`
	Expected       string            `json:"expected"`
	ExpectError    string            `json:"expect_error"`
}

type repeatSpec struct {
	Text  string `json:"text"`
	Times int    `json:"times"`
}

type conformanceResult struct {
	ID       string
	Passed   bool
	Expected string
	Actual   string
	Note     string
}

// runConformance 执行模板渲染一致性测试向量。
// 返回退出码：0 全通过，1 有失败。
func runConformance(vectorsPath string, verbose bool) int {
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取测试向量失败：%v\n", err)
		return 2
	}

	var cases []conformanceCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		fmt.Fprintf(os.Stderr, "解析测试向量失败：%v\n", err)
		return 2
	}
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "测试向量为空")
		return 2
	}

	fmt.Printf("模板渲染一致性测试 · 规范 v1 · 向量 %d 条\n\n", len(cases))

	results := make([]conformanceResult, 0, len(cases))
	for _, c := range cases {
		results = append(results, runOneCase(c))
	}

	failed := 0
	for _, r := range results {
		if r.Passed {
			fmt.Printf("  PASS  %-30s %s\n", r.ID, r.Note)
			if verbose && r.Note != "" {
				continue
			}
			continue
		}
		failed++
		fmt.Printf("  FAIL  %-30s\n", r.ID)
		fmt.Printf("        expected: %s\n", oneLine(r.Expected))
		fmt.Printf("        actual  : %s\n", oneLine(r.Actual))
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	fmt.Printf("\n结果：%d/%d 通过\n", len(results)-failed, len(results))
	if failed > 0 {
		fmt.Println("未全部通过 → 该实现不得声明支持 block_templates")
		return 1
	}
	fmt.Println("全部通过 → 可声明 block_templates: true")
	return 0
}

func runOneCase(c conformanceCase) conformanceResult {
	tpl := c.Template
	if c.TemplateRepeat != nil {
		tpl = strings.Repeat(c.TemplateRepeat.Text, c.TemplateRepeat.Times)
	}

	got, err := renderTemplate(tpl, c.Vars)

	if c.ExpectError != "" {
		if err == nil {
			return conformanceResult{
				ID: c.ID, Passed: false,
				Expected: "error:" + c.ExpectError,
				Actual:   oneLine(got),
			}
		}
		if err.Error() != c.ExpectError {
			return conformanceResult{
				ID: c.ID, Passed: false,
				Expected: "error:" + c.ExpectError,
				Actual:   "error:" + err.Error(),
			}
		}
		return conformanceResult{ID: c.ID, Passed: true, Note: "按预期返回 " + c.ExpectError}
	}

	if err != nil {
		return conformanceResult{
			ID: c.ID, Passed: false,
			Expected: oneLine(c.Expected),
			Actual:   "error:" + err.Error(),
		}
	}
	if got != c.Expected {
		return conformanceResult{
			ID: c.ID, Passed: false,
			Expected: c.Expected,
			Actual:   got,
		}
	}
	return conformanceResult{ID: c.ID, Passed: true, Note: c.Description}
}

// oneLine 把多行/含控制字符的输出压成单行，便于在终端比对。
func oneLine(s string) string {
	if s == "" {
		return "(空)"
	}
	r := strings.NewReplacer("\r\n", "\\r\\n", "\n", "\\n", "\r", "\\r", "\t", "\\t")
	out := r.Replace(s)
	if len(out) > 160 {
		out = out[:160] + "…(已截断)"
	}
	return out
}
