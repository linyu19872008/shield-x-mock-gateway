# 模板渲染一致性测试向量

> 规范来源：[gateway 接入契约 v1 · §9](../../docs/contract/gateway-contract-v1.md#9-拦截模板渲染规范-v1)
> 用例文件：[`cases.json`](cases.json) · 共 **14** 条
> 适用对象：**任何语言的 gateway 实现**，以及 shield-x 自身的预览渲染器

---

## 1. 这是什么

拦截模板的 HTML 由 **gateway 本地渲染**（shield-x 不在请求路径上），而模板的编辑与**预览在 shield-x 后台**。

这意味着同一份模板会被**两套代码**渲染。只要转义规则、占位符处理有任何一处不一致，就会出现 **"后台预览正常、线上白屏"**。

本目录用一组 **`(模板, 变量, 期望输出)`** 的用例，把渲染规范从文字变成**可执行断言**。

> **通过全部 14 条 = 可以声明支持 `block_templates: true`。**
> 有一条不过，就必须声明 `block_templates: false`，否则线上一定出问题。

---

## 2. 文件格式

`cases.json` 是一个 JSON 数组，每个元素是一条用例。

```json
{
  "id": "01-simple-replace",
  "description": "给人看的中文说明",
  "template": "<p>{{request_id}}</p>",
  "vars": { "request_id": "req-001" },
  "expected": "<p>req-001</p>"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 唯一标识，kebab-case，数字前缀表示顺序 |
| `description` | string | 验证点说明 |
| `template` | string | 模板源码 |
| `template_repeat` | object | 与 `template` 二选一。`{text, times}`，运行器展开为 `text` 重复 `times` 次（用于"超大模板"这类不方便内联的用例） |
| `vars` | object | 占位符取值。**7 个白名单占位符全部从这里取**，不区分"来自请求"还是"来自模板变量" |
| `expected` | string | 期望输出（逐字节比较） |
| `expect_error` | string | 与 `expected` 二选一。期望渲染失败，值为错误码 |

**错误码：**

| 码 | 含义 |
|---|---|
| `TEMPLATE_TOO_LARGE` | 模板 UTF-8 编码后超过 65536 字节 |

---

## 3. 渲染规则（来自契约 §9，此处为运行器视角）

```
1. 若模板以 U+FEFF 开头 → 剔除 BOM
2. 若 UTF-8 字节数 > 65536 → 返回 TEMPLATE_TOO_LARGE
3. 用正则 \{\{\s*([a-z_]+)\s*\}\} 从左到右单遍扫描：
     a. 捕获组在白名单内 → 用「HTML 转义后的变量值」替换整个匹配
     b. 不在白名单       → 整个匹配原样保留（含内部空白）
     c. 变量缺失/null    → 替换为空串
4. 替换结果不参与二次扫描（单遍，无递归）
```

**白名单（7 个）：**
`request_id` · `timestamp` · `client_ip` · `host` · `path` · `site_name` · `contact_email`

**HTML 转义（顺序不可换）：**

| 步骤 | 替换 |
|---|---|
| 1 | `&` → `&amp;` |
| 2 | `<` → `&lt;` |
| 3 | `>` → `&gt;` |
| 4 | `"` → `&quot;` |
| 5 | `'` → `&#39;` |

---

## 4. 用例清单

| # | id | 验证点 |
|---|---|---|
| 01 | `simple-replace` | 基础替换 |
| 02 | `multi-placeholders` | 多占位符混排互不干扰 |
| 03 | `escape-special-chars` | 五个特殊字符全部转义（防模板注入 / 反射型 XSS） |
| 04 | `escape-amp-order` | **`&` 必须先转义**，否则 `<` 会被二次转义 |
| 05 | `unknown-kept` | 未知占位符原样保留（含内部空白） |
| 06 | `missing-var-empty` | 变量缺失 → 空串，不抛错 |
| 07 | `css-braces` | 内联 CSS 的 `{}` 不被误判 |
| 08 | `nested-braces` | `{{{{request_id}}}}` 的确定性行为：`{{r-8}}` |
| 09 | `unicode` | 中文 / emoji / IPv6 透传 |
| 10 | `crlf-preserved` | CRLF 不得被统一成 LF |
| 11 | `bom-stripped` | 开头 BOM 必须剔除 |
| 12 | `too-large` | > 64 KiB 拒绝渲染 |
| 13 | `whitespace-in-placeholder` | `{{   request_id   }}` 合法 |
| 14 | `uppercase-not-matched` | 占位符名只允许小写，`{{Request_ID}}` 原样保留 |

### 4.1 两条特别值得注意的用例

**04 `escape-amp-order`**

输入 `a<b`：
- 正确（先转 `&`，再转 `<`）：`a&lt;b`
- 错误（先转 `<`，再转 `&`）：`a&amp;lt;b` ← 会多出一层

这是最容易写错、也最容易被忽略的顺序问题。

**13 `whitespace-in-placeholder`**

模板里 `{{   request_id   }}` 必须合法。
**用"逐 key 字符串替换"（`strings.ReplaceAll(tpl, "{{request_id}}", v)`）的实现会在这条失败**——这是最常见的偷懒写法，也是本向量存在的直接原因。

---

## 5. 如何运行

### 5.1 用 mock-gateway（推荐）

```bash
mock-gateway conformance --vectors ./testdata/template-conformance/cases.json
```

```
PASS 01-simple-replace
PASS 02-multi-placeholders
...
FAIL 13-whitespace-in-placeholder
      expected: <p>r-13</p>
      actual  : <p>{{   request_id   }}</p>
结果: 13/14 通过
```

退出码：`0` 全通过，`1` 有失败。

### 5.2 其他语言

自行实现一个几十行的 runner：

1. 读 `cases.json`
2. 对每条：若含 `template_repeat` 先展开；调用本语言的渲染器
3. `expect_error` 存在时断言抛错码，否则逐字节比对 `expected`
4. 全通过 → 退出 0

**注意：JSON 解析后字符串里的 `\r\n`、`\uFEFF` 是真实字符，不要额外再转义一次。**

### 5.3 手工核对

任何一条用例都可以用任意语言的 REPL 单独验证，不需要依赖 shield-x。

---

## 6. 常见实现错误（自查清单）

| 错误写法 | 会在哪条失败 |
|---|---|
| 逐 key 做 `ReplaceAll`，而不是正则扫描 | 13（空白）、14（大写） |
| 先转义 `<` 再转义 `&` | 04 |
| 未知占位符替换成空串 | 05 |
| 未知占位符整段删除 | 05 |
| 变量缺失时抛异常/留占位符 | 06 |
| 用 `str_replace` 处理 CSS 里的 `{}` | 07 |
| 用 `TrimSpace` 或换行归一化处理模板 | 10 |
| 不去 BOM | 11 |
| 没有大小上限 | 12 |
| 对替换结果再跑一遍替换（递归） | 08 |
| 只匹配大写/混合大小写的占位符 | 14 |

---

## 7. 变更流程

新增或修改测试向量时：

1. **先改规范**（契约 §9），再改向量——向量是规范的镜像，不是规范本身
2. 新增占位符 → 同时把 `template_spec_version` +1
3. 同步更新 `mock-gateway` 的渲染器与 shield-x 后台预览的渲染器
4. 通知所有已接入 gateway 的团队：**低于新 spec version 的实现将不再被判定为"支持 block_templates"**
5. 本文件与 `cases.json` 同一次提交内更新

---

## 8. 已知边界（不在本向量覆盖范围）

| 项 | 原因 |
|---|---|
| 禁用标签校验（`<script>` / `<iframe>`） | 属于**保存期校验**，不是渲染规范，见契约 §12.4 |
| 自定义响应头白名单 | 同上 |
| 大模板渲染性能 | 不属于正确性 |
| 并发安全 | 由各语言实现自行保证（渲染器应为无状态纯函数） |
