package template

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxTemplateBytes 与 shield-x 契约 §9 保持一致：64 KiB。
const maxTemplateBytes = 64 * 1024

// ErrTemplateTooLarge 对应测试向量里的错误码 TEMPLATE_TOO_LARGE。
var ErrTemplateTooLarge = errors.New("TEMPLATE_TOO_LARGE")

// 占位符语法：{{ name }}，name 只允许小写字母与下划线。
// 注意这里刻意不接受大写、数字、表达式——规范要求「严格白名单 + 单遍替换」。
var placeholderRe = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

// 白名单：只有这些名字会被替换，其余原样保留。
var placeholderWhitelist = map[string]struct{}{
	"request_id":    {},
	"timestamp":     {},
	"client_ip":     {},
	"host":          {},
	"path":          {},
	"site_name":     {},
	"contact_email": {},
}

// renderTemplate 独立实现 shield-x 契约 §9 的模板渲染。
//
// 这个实现**刻意不 import shield-x 的 internal 包**：
// mock-gateway 的身份是「契约的第一个第三方实现」，一旦复用同一份代码，
// template-conformance 就变成了自证同义反复，测不出规范歧义。
func Render(tpl string, vars map[string]string) (string, error) {
	// 1. 剔除 UTF-8 BOM
	tpl = strings.TrimPrefix(tpl, "\uFEFF")

	// 2. 大小上限（按 UTF-8 字节数）
	if !utf8.ValidString(tpl) {
		return "", errors.New("TEMPLATE_INVALID_UTF8")
	}
	if len(tpl) > maxTemplateBytes {
		return "", ErrTemplateTooLarge
	}

	// 3. 单遍替换：替换结果不再参与扫描
	out := placeholderRe.ReplaceAllStringFunc(tpl, func(matched string) string {
		sub := placeholderRe.FindStringSubmatch(matched)
		if len(sub) != 2 {
			return matched
		}
		key := sub[1]
		if _, ok := placeholderWhitelist[key]; !ok {
			return matched // 未知占位符：整段原样保留
		}
		return escapeHTML(vars[key])
	})

	return out, nil
}

// escapeHTML 按契约规定的顺序转义：& 必须最先处理，否则会二次转义。
func escapeHTML(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// builtinBlockPage 是兜底拦截页：模板缺失 / 校验失败 / 渲染异常时使用。
//
// 真实 gateway 同样必须内置一份，否则模板故障会导致拦截动作返回白屏。
const builtinBlockPage = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">` +
	`<title>访问被拦截</title></head><body><h1>访问被拦截</h1>` +
	`<p>请求编号：{{request_id}}</p></body></html>`

func renderBuiltin(requestID string) string {
	out, err := Render(builtinBlockPage, map[string]string{"request_id": requestID})
	if err != nil {
		// 内置页内容固定且远小于上限，不可能失败；真失败说明代码被改坏了。
		return fmt.Sprintf("<h1>Blocked</h1><p>%s</p>", escapeHTML(requestID))
	}
	return out
}
