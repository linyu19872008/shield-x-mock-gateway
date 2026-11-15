(function () {
  "use strict";

  var PAGES = {
    overview: { title: "概览", sub: "运行状态与吞吐" },
    connection: { title: "连接与能力", sub: "shield-x 接入参数与能力声明" },
    policies: { title: "策略与模板", sub: "当前快照与模板缓存" },
    traffic: { title: "流量生成", sub: "启动 / 停止与速率控制" },
    fault: { title: "故障注入", sub: "模拟 shield-x 不可达与异常" },
    spool: { title: "上报与 Spool", sub: "队列、重试与磁盘补发" },
    logs: { title: "日志与配置", sub: "环形日志与脱敏配置" }
  };

  var state = {
    page: "overview",
    status: null,
    config: null,
    policies: null,
    templates: [],
    logs: [],
    logFilter: "",
    token: localStorage.getItem("mock_token") || "",
    authOpen: false,
    online: true,
    theme: localStorage.getItem("mock_theme") || "auto"
  };

  var $ = function (sel, root) { return (root || document).querySelector(sel); };
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); };
  var view = $("#view");

  function esc(value) {
    return String(value == null ? "" : value).replace(/[&<>"']/g, function (ch) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch];
    });
  }

  function num(value) {
    var n = Number(value || 0);
    return Number.isFinite(n) ? n.toLocaleString("en-US") : "0";
  }

  function fmtBytes(n) {
    n = Number(n || 0);
    if (n < 1024) return n + " B";
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KiB";
    return (n / 1024 / 1024).toFixed(1) + " MiB";
  }

  function fmtTime(value) {
    if (!value) return "—";
    var d = new Date(value);
    if (isNaN(d.getTime()) || d.getFullYear() < 2000) return "—";
    return d.toLocaleString("zh-CN", { hour12: false });
  }

  function fmtClock(value) {
    if (!value) return "";
    var d = new Date(value);
    if (isNaN(d.getTime())) return "";
    return d.toLocaleTimeString("zh-CN", { hour12: false });
  }

  function fmtUptime(seconds) {
    var s = Math.max(0, Number(seconds || 0));
    if (s < 60) return Math.round(s) + "s";
    var m = Math.floor(s / 60);
    if (m < 60) return m + "m " + Math.round(s % 60) + "s";
    var h = Math.floor(m / 60);
    return h + "h " + (m % 60) + "m";
  }

  function pct(a, b) {
    if (!b) return "0%";
    return Math.round((a / b) * 100) + "%";
  }

  var toastTimer = null;
  function toast(message, kind) {
    var el = $("#toast");
    el.textContent = message;
    el.className = "toast" + (kind === "error" ? " error" : "");
    el.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { el.hidden = true; }, 3400);
  }

  function openAuth() {
    state.authOpen = true;
    $("#auth").hidden = false;
    $("#auth-token").value = "";
    setTimeout(function () { $("#auth-token").focus(); }, 30);
  }

  async function api(path, options) {
    options = options || {};
    var headers = new Headers(options.headers || {});
    if (state.token) headers.set("X-Mock-Token", state.token);
    if (options.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");

    var res;
    try {
      res = await fetch(path, {
        method: options.method || "GET",
        headers: headers,
        body: options.body
      });
    } catch (err) {
      throw new Error("无法连接控制台：" + err.message);
    }

    if (res.status === 401) {
      openAuth();
      throw new Error("需要控制台 Token");
    }

    var text = await res.text();
    var data = null;
    if (text) {
      try { data = JSON.parse(text); } catch (err) { data = null; }
    }
    if (!res.ok) {
      var msg = data && data.error && data.error.message ? data.error.message : "HTTP " + res.status;
      throw new Error(msg);
    }
    return data;
  }

  async function ensureConfig(force) {
    if (state.config && !force) return state.config;
    state.config = await api("/api/config");
    return state.config;
  }

  async function loadPolicies(force) {
    if (state.policies && !force) return;
    var result = await Promise.all([api("/api/policies"), api("/api/templates")]);
    state.policies = result[0];
    state.templates = (result[1] && result[1].items) || [];
  }

  async function loadLogs(silent) {
    try {
      var data = await api("/api/logs?limit=300");
      state.logs = (data && data.items) || [];
      if (state.page === "logs") renderLogList();
    } catch (err) {
      if (!silent) toast(err.message, "error");
    }
  }

  function applyTheme(theme) {
    state.theme = theme;
    localStorage.setItem("mock_theme", theme);
    if (theme === "auto") {
      document.documentElement.removeAttribute("data-theme");
    } else {
      document.documentElement.setAttribute("data-theme", theme);
    }
    var label = theme === "auto" ? "自动" : theme === "light" ? "浅色" : "深色";
    $("#theme-btn").textContent = "外观：" + label;
  }

  function renderChrome() {
    var pill = $("#run-pill");
    var stopBtn = $("#stop-btn");
    if (!state.online) {
      pill.className = "pill bad";
      pill.innerHTML = '<span class="dot"></span>控制台离线';
      stopBtn.disabled = true;
      return;
    }
    var s = state.status;
    if (!s) {
      pill.className = "pill";
      pill.innerHTML = '<span class="dot"></span>连接中…';
      stopBtn.disabled = true;
      return;
    }
    if (s.running) {
      pill.className = "pill ok";
      pill.innerHTML = '<span class="dot"></span>运行中 · ' + esc(s.mode || "run");
      stopBtn.disabled = false;
    } else {
      pill.className = "pill";
      pill.innerHTML = '<span class="dot"></span>未运行';
      stopBtn.disabled = true;
    }
  }

  async function refreshStatus() {
    if (state.authOpen) return;
    try {
      state.status = await api("/api/status");
      state.online = true;
      renderChrome();
      if (!state.config) {
        try { await ensureConfig(false); } catch (err) { /* 配置读取失败不阻塞状态 */ }
      }
      if (state.page === "overview") renderOverview();
      else if (state.page === "spool") renderSpool();
      else if (state.page === "traffic") renderTrafficStatus();
    } catch (err) {
      if (err.message !== "需要控制台 Token") {
        state.online = false;
        renderChrome();
      }
    }
  }

  function loadingBlock() {
    return '<div class="loading">加载中…</div>';
  }

  function errorBlock(message) {
    return '<div class="card"><div class="callout error"><strong>加载失败：</strong>' + esc(message) + "</div></div>";
  }

  function emptyBlock(title, body) {
    return '<div class="empty"><strong>' + esc(title) + "</strong>" + esc(body) + "</div>";
  }

  function statCard(label, value, foot) {
    return '<div class="card stat"><div class="stat-label">' + esc(label) +
      '</div><div class="stat-value">' + esc(value) +
      '</div><div class="stat-foot">' + esc(foot) + "</div></div>";
  }

  function renderOverview() {
    var s = state.status;
    if (!s) {
      view.innerHTML = loadingBlock();
      return;
    }
    var running = !!s.running;
    var modeLabel = s.mode === "loadtest" ? "压测" : "常驻模拟";
    var gateway = state.config ? state.config.gateway_id : "";

    var cards = [
      ["已生成", num(s.generated), "generator 产出"],
      ["已入队", num(s.enqueued), "进入上报队列"],
      ["已发批次", num(s.sent), "HTTP 请求次数"],
      ["服务端接收", num(s.accepted), "accepted"],
      ["拒绝", num(s.rejected), "rejected"],
      ["客户端丢弃", num(s.dropped), "队列满 / 非法事件"],
      ["队列深度", num(s.queue_depth), "待上报事件"],
      ["Spool", num(s.spool_events), "待补发事件"]
    ].map(function (item) { return statCard(item[0], item[1], item[2]); }).join("");

    var pollTotal = Number(s.poll_success || 0) + Number(s.poll_not_modified || 0);
    var lastError = s.last_error
      ? '<div class="callout error"><strong>最近错误：</strong>' + esc(s.last_error) + "</div>"
      : '<div class="callout">最近没有错误。</div>';

    view.innerHTML =
      '<section class="card hero">' +
        '<div>' +
          '<span class="eyebrow">' + (running ? "运行中" : "空闲") + "</span>" +
          "<h2>" + (running ? modeLabel + "进行中" : "mock-gateway 已就绪") + "</h2>" +
          "<p>" + (running
            ? "已运行 " + fmtUptime(s.uptime_seconds) + "，gateway " + esc(gateway || "—") + "。事件经 shield-x-client-go 上报。"
            : "在「流量生成」页选择 run 或 loadtest 启动；求值、渲染与流量生成由 mock 独立完成。") +
          "</p>" +
        "</div>" +
        '<div class="hero-meta">' +
          "<div><span>revision</span><strong>" + num(s.revision) + "</strong></div>" +
          "<div><span>policies</span><strong>" + num(s.policies) + "</strong></div>" +
          "<div><span>templates</span><strong>" + num(s.templates) + "</strong></div>" +
        "</div>" +
      "</section>" +
      '<div class="grid stats">' + cards + "</div>" +
      '<div class="grid two">' +
        '<section class="card">' +
          '<div class="card-head"><div><h2>轮询与快照</h2><p>SDK poller 的 200 / 304 与健康度</p></div></div>' +
          '<dl class="kv">' +
            "<dt>200 成功</dt><dd>" + num(s.poll_success) + "</dd>" +
            "<dt>304 未变更</dt><dd>" + num(s.poll_not_modified) + "（" + esc(pct(s.poll_not_modified, pollTotal)) + "）</dd>" +
            "<dt>失败</dt><dd>" + num(s.poll_failures) + "</dd>" +
            "<dt>当前 revision</dt><dd>" + num(s.revision) + "</dd>" +
            "<dt>策略 / 模板</dt><dd>" + num(s.policies) + " / " + num(s.templates) + "</dd>" +
          "</dl>" +
        "</section>" +
        '<section class="card">' +
          '<div class="card-head"><div><h2>最近错误</h2><p>来自 SDK 的 poller / reporter</p></div></div>' +
          lastError +
        "</section>" +
      "</div>";
  }

  function renderConnection() {
    var cfg = state.config;
    if (!cfg) {
      view.innerHTML = loadingBlock();
      return;
    }
    var caps = cfg.capabilities || {};
    var actions = (caps.actions || []).map(function (a) { return '<span class="chip">' + esc(a) + "</span>"; }).join("");
    var fields = (caps.match_fields || []).map(function (a) { return '<span class="chip">' + esc(a) + "</span>"; }).join("");
    var fault = cfg.fault || {};

    view.innerHTML =
      '<section class="card">' +
        '<div class="card-head"><div><h2>接入配置</h2><p>来自启动时的配置；修改需重启 serve</p></div></div>' +
        '<dl class="kv">' +
          "<dt>Base URL</dt><dd>" + esc(cfg.base_url) + "</dd>" +
          "<dt>Gateway ID</dt><dd>" + esc(cfg.gateway_id) + "</dd>" +
          "<dt>Agent Version</dt><dd>" + esc(cfg.agent_version) + "</dd>" +
          "<dt>租户 Token</dt><dd>" + (cfg.token_set ? "已配置（不回显）" : "未配置") + "</dd>" +
          "<dt>拉策略周期</dt><dd>" + esc(cfg.poll_interval) + "</dd>" +
          "<dt>上报周期 / 格式</dt><dd>" + esc(cfg.report_interval) + " · " + esc(cfg.report_format) + "</dd>" +
          "<dt>Spool 目录</dt><dd>" + esc(cfg.spool_dir || "未启用") + "</dd>" +
        "</dl>" +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>能力声明</h2><p>随拉策略请求发送，服务端据此过滤策略</p></div></div>' +
        '<div class="chips" style="margin-bottom:12px">' + (actions || '<span class="muted small">未声明动作</span>') + "</div>" +
        '<div class="chips" style="margin-bottom:12px">' + (fields || '<span class="muted small">未声明匹配字段</span>') + "</div>" +
        '<dl class="kv">' +
          "<dt>自定义模板</dt><dd>" + (caps.block_templates ? "支持" : "不支持") + "</dd>" +
          "<dt>模板规范版本</dt><dd>" + num(caps.template_spec_version) + "</dd>" +
          "<dt>最大策略数</dt><dd>" + num(caps.max_rules) + "</dd>" +
        "</dl>" +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>故障注入配置</h2><p>运行时仅「不可达」可切换，其余需改配置并重启</p></div></div>' +
        '<dl class="kv">' +
          "<dt>shield-x 不可达</dt><dd>" + (fault.shieldx_down ? "开启" : "关闭") + "</dd>" +
          "<dt>注入延迟</dt><dd>" + esc(fault.latency || "0s") + "</dd>" +
          "<dt>丢包比例</dt><dd>" + num(fault.drop_percent) + "%</dd>" +
          "<dt>状态码覆盖</dt><dd>" + (fault.http_status_override ? num(fault.http_status_override) : "未开启") + "</dd>" +
        "</dl>" +
      "</section>";
  }

  function policyActionClass(action) {
    return ["block", "observe", "redirect"].indexOf(action) >= 0 ? " " + action : "";
  }

  function renderPolicies() {
    var snap = state.policies || { policies: [] };
    var policies = snap.policies || [];
    var templates = state.templates || [];

    var header =
      '<section class="card">' +
        '<div class="card-head"><div><h2>策略快照</h2><p>revision ' + num(snap.revision) +
          " · 拉取于 " + esc(fmtTime(snap.fetched_at)) + " · 服务端 " + esc(snap.server || "—") + "</p></div>" +
          '<button id="refresh-policies" class="btn btn-ghost" type="button">刷新</button></div>';

    var body = policies.length
      ? policies.map(function (p) {
          var action = (p.action && p.action.type) || "none";
          return '<details class="policy">' +
            "<summary>" +
              '<span class="prio">' + num(p.priority) + "</span>" +
              '<span class="policy-name">' + esc(p.name || "未命名策略") + "</span>" +
              '<span class="policy-id">' + esc(p.id) + "</span>" +
              '<span class="spacer"></span>' +
              '<span class="badge' + policyActionClass(action) + '">' + esc(action) + "</span>" +
            "</summary>" +
            "<pre>" + esc(JSON.stringify({ match: p.match, action: p.action }, null, 2)) + "</pre>" +
          "</details>";
        }).join("")
      : emptyBlock("当前没有策略", "租户策略为空时，mock 会按空快照运行。");

    var tplBody = templates.length
      ? '<div class="tpl-grid">' + templates.map(function (t) {
          var vars = Object.keys(t.vars || {}).map(function (k) {
            return '<span class="chip">' + esc(k) + "</span>";
          }).join("");
          var headers = Object.keys(t.headers || {}).map(function (k) {
            return '<span class="chip">' + esc(k) + "</span>";
          }).join("");
          return '<article class="tpl-card">' +
            "<header><h3>" + esc(t.name || t.id) + "</h3>" +
              '<p class="meta">' + esc(t.id) + " · v" + num(t.version) + "</p>" +
              '<p class="meta">' + esc(t.content_type || "") + " · " + num(t.status_code) + "</p></header>" +
            '<pre class="tpl-preview">' + esc(t.content || "") + "</pre>" +
            "<footer>" + (vars || headers
              ? '<div class="chips">' + vars + headers + "</div>"
              : '<span class="muted small">无变量 / 自定义头</span>') + "</footer>" +
          "</article>";
        }).join("") + "</div>"
      : emptyBlock("当前没有模板", "策略引用到缺失模板时，mock 会触发一次模板拉取。");

    view.innerHTML = header + body +
      '<div class="card-head" style="margin-top:16px"><div><h2>拦截模板</h2><p>来自 SDK 的模板快照</p></div></div>' +
      tplBody + "</section>";

    var btn = $("#refresh-policies");
    if (btn) {
      btn.addEventListener("click", async function () {
        btn.disabled = true;
        btn.textContent = "刷新中…";
        try {
          await loadPolicies(true);
          renderPolicies();
          toast("策略与模板已刷新");
        } catch (err) {
          toast(err.message, "error");
          btn.disabled = false;
          btn.textContent = "刷新";
        }
      });
    }
  }

  function renderTraffic() {
    var cfg = state.config || {};
    var rate = Number(cfg.rate || 20);
    view.innerHTML =
      '<section class="card">' +
        '<div class="card-head"><div><h2>启动模拟</h2><p>run 常驻；loadtest 到点自动停止并 flush</p></div></div>' +
        '<p class="hint" style="margin:0 0 12px">表单用于下一次启动；当前运行速率见下方状态。</p>' +
        '<div class="form-grid">' +
          '<label class="field">模式<select id="traffic-mode">' +
            '<option value="run">run · 常驻</option>' +
            '<option value="loadtest">loadtest · 定时</option>' +
          "</select></label>" +
          '<label class="field">速率 events/s<input id="traffic-rate" type="number" min="1" value="' + rate + '"></label>' +
          '<label class="field">时长（loadtest）<input id="traffic-duration" type="text" value="30s" disabled></label>' +
        "</div>" +
        '<div class="row-actions">' +
          '<button id="traffic-start" class="btn btn-primary" type="button">启动</button>' +
          '<button id="traffic-stop" class="btn btn-ghost" type="button">停止</button>' +
          '<span class="hint">预设：</span>' +
          '<button class="btn btn-ghost" type="button" data-rate="20">20</button>' +
          '<button class="btn btn-ghost" type="button" data-rate="200">200</button>' +
          '<button class="btn btn-ghost" type="button" data-rate="1000">1000</button>' +
          '<button class="btn btn-ghost" type="button" data-rate="3000">3000</button>' +
        "</div>" +
        '<div id="traffic-state" class="inline-status"></div>' +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>说明</h2></div></div>' +
        '<div class="callout">loadtest 在时长到达后由 Controller 自动取消运行；SDK reporter 会按 ShutdownTimeout 做最后一次 flush。' +
        "生成速率是目标值，实际达成率受本机 CPU 与网络影响。</div>" +
      "</section>";

    var mode = $("#traffic-mode");
    var duration = $("#traffic-duration");
    function syncDuration() {
      duration.disabled = mode.value !== "loadtest";
    }
    mode.addEventListener("change", syncDuration);
    syncDuration();

    $$("[data-rate]").forEach(function (btn) {
      btn.addEventListener("click", function () { $("#traffic-rate").value = btn.getAttribute("data-rate"); });
    });

    $("#traffic-start").addEventListener("click", startFlow);
    $("#traffic-stop").addEventListener("click", stopFlow);
    renderTrafficStatus();
  }

  function renderTrafficStatus() {
    var el = $("#traffic-state");
    if (!el) return;
    var s = state.status;
    if (!s) {
      el.textContent = "";
      return;
    }
    el.textContent = s.running
      ? "运行中：" + (s.mode || "run") + " · " + num(s.rate) + "/s · 已生成 " + num(s.generated) +
        " · accepted " + num(s.accepted) + " · 队列 " + num(s.queue_depth)
      : "当前未运行。";
  }

  async function startFlow() {
    var modeEl = $("#traffic-mode");
    var rateEl = $("#traffic-rate");
    var durEl = $("#traffic-duration");
    if (!modeEl || !rateEl) return;

    var rate = Number(rateEl.value);
    if (!Number.isFinite(rate) || rate < 1) {
      toast("速率必须是正整数", "error");
      return;
    }
    var mode = modeEl.value;
    var duration = mode === "loadtest" ? durEl.value.trim() : "";
    if (mode === "loadtest" && !duration) {
      toast("loadtest 需要填写时长，例如 30s", "error");
      return;
    }

    var btn = $("#traffic-start");
    btn.disabled = true;
    btn.textContent = "启动中…";
    try {
      await api("/api/start", {
        method: "POST",
        body: JSON.stringify({ mode: mode, rate: rate, duration: duration })
      });
      toast("已启动：" + mode);
      await refreshStatus();
    } catch (err) {
      toast(err.message, "error");
    } finally {
      btn.disabled = false;
      btn.textContent = "启动";
    }
  }

  async function stopFlow() {
    try {
      await api("/api/stop", { method: "POST", body: "{}" });
      toast("已停止");
      await refreshStatus();
    } catch (err) {
      toast(err.message, "error");
    }
  }

  function renderFault() {
    var cfg = state.config;
    if (!cfg) {
      view.innerHTML = loadingBlock();
      return;
    }
    var fault = cfg.fault || {};
    view.innerHTML =
      '<section class="card">' +
        '<div class="card-head"><div><h2>运行时开关</h2><p>立即生效，不需要重启</p></div></div>' +
        '<label class="switch">' +
          '<input id="fault-down" type="checkbox"' + (fault.shieldx_down ? " checked" : "") + ">" +
          '<span class="switch-track"></span>' +
          "<span>模拟 shield-x 不可达</span>" +
        "</label>" +
        '<div class="callout warn" style="margin-top:14px">开启后，SDK 的拉策略/上报都会失败：poller 保留旧快照并按退避重试，reporter 会把批次写入 spool（若配置了目录）。</div>' +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>静态注入配置</h2><p>来自配置文件，修改后需重启 serve</p></div></div>' +
        '<dl class="kv">' +
          "<dt>注入延迟</dt><dd>" + esc(fault.latency || "0s") + "</dd>" +
          "<dt>丢包比例</dt><dd>" + num(fault.drop_percent) + "%</dd>" +
          "<dt>状态码覆盖</dt><dd>" + (fault.http_status_override ? num(fault.http_status_override) : "未开启") + "</dd>" +
        "</dl>" +
      "</section>";

    var input = $("#fault-down");
    input.addEventListener("change", async function () {
      var down = input.checked;
      input.disabled = true;
      try {
        await api("/api/fault", { method: "POST", body: JSON.stringify({ down: down }) });
        fault.shieldx_down = down;
        toast(down ? "已切换为不可达" : "已恢复连接");
        await refreshStatus();
      } catch (err) {
        input.checked = !down;
        toast(err.message, "error");
      } finally {
        input.disabled = false;
      }
    });
  }

  function renderSpool() {
    var s = state.status;
    var cfg = state.config || {};
    if (!s) {
      view.innerHTML = loadingBlock();
      return;
    }
    view.innerHTML =
      '<div class="grid three">' +
        statCard("待补发事件", num(s.spool_events), "spool 中的事件数") +
        statCard("文件数", num(s.spool_files), "磁盘批次文件") +
        statCard("占用空间", fmtBytes(s.spool_bytes), "spool 目录") +
      "</div>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>补发策略</h2><p>由 SDK reporter 管理</p></div></div>' +
        '<div class="callout">' +
          "上报失败（5xx / 超时 / 网络）时，SDK 会把批次写入 <code>" + esc(cfg.spool_dir || "（未配置）") +
          "</code>；Run 启动时与恢复成功后自动补发。413 会先拆半，400 直接丢弃并计数。" +
          "本页只读；清空目录请在确认后手动操作。" +
        "</div>" +
        '<dl class="kv" style="margin-top:12px">' +
          "<dt>队列深度</dt><dd>" + num(s.queue_depth) + "</dd>" +
          "<dt>已落盘</dt><dd>" + num(s.spooled) + "（累计）</dd>" +
          "<dt>已补发</dt><dd>" + num(s.replayed) + "（累计）</dd>" +
        "</dl>" +
      "</section>";
  }

  function renderLogs() {
    view.innerHTML =
      '<section class="card">' +
        '<div class="card-head"><div><h2>环形日志</h2><p>最近 500 条，包含 SDK 的 poller / reporter 错误</p></div></div>' +
        '<div class="toolbar">' +
          '<input id="log-filter" class="filter" placeholder="过滤关键字" value="' + esc(state.logFilter) + '">' +
          '<button id="refresh-logs" class="btn btn-ghost" type="button">刷新</button>' +
        '</div>' +
        '<div id="log-list"></div>' +
      "</section>" +
      '<details class="card">' +
        "<summary><strong>脱敏配置 JSON</strong> <span class=\"muted small\">点击展开</span></summary>" +
        '<pre class="mono small" style="margin:10px 0 0;overflow:auto">' + esc(JSON.stringify(state.config || {}, null, 2)) + "</pre>" +
      "</details>";

    var filter = $("#log-filter");
    filter.addEventListener("input", function () {
      state.logFilter = filter.value;
      renderLogList();
    });
    $("#refresh-logs").addEventListener("click", function () { loadLogs(false); });
    renderLogList();
  }

  function renderLogList() {
    var box = $("#log-list");
    if (!box) return;
    var needle = state.logFilter.trim().toLowerCase();
    var items = state.logs.filter(function (item) {
      return !needle || String(item.message || "").toLowerCase().indexOf(needle) >= 0;
    });
    if (!items.length) {
      box.innerHTML = emptyBlock("没有匹配的日志", state.logFilter ? "换个关键字试试。" : "运行 mock 后这里会出现轮询/上报日志。");
      return;
    }
    box.innerHTML = '<div class="logs">' + items.map(function (item) {
      return '<div class="log-row"><span class="log-time">' + esc(fmtClock(item.time)) +
        '</span><span class="log-msg">' + esc(item.message) + "</span></div>";
    }).join("") + "</div>";
  }

  async function renderPage(page) {
    try {
      if (page === "overview") {
        renderOverview();
      } else if (page === "connection") {
        await ensureConfig(false);
        if (state.page !== page) return;
        renderConnection();
      } else if (page === "policies") {
        await loadPolicies(false);
        if (state.page !== page) return;
        renderPolicies();
      } else if (page === "traffic") {
        await ensureConfig(false);
        if (state.page !== page) return;
        renderTraffic();
      } else if (page === "fault") {
        await ensureConfig(false);
        if (state.page !== page) return;
        renderFault();
      } else if (page === "spool") {
        await ensureConfig(false);
        if (state.page !== page) return;
        renderSpool();
      } else if (page === "logs") {
        await ensureConfig(false);
        await loadLogs(true);
        if (state.page !== page) return;
        renderLogs();
      }
    } catch (err) {
      if (state.page === page) view.innerHTML = errorBlock(err.message);
    }
  }

  function navigate(page, updateHash) {
    if (!PAGES[page]) page = "overview";
    state.page = page;
    $$(".nav-item").forEach(function (btn) {
      if (btn.getAttribute("data-page") === page) btn.setAttribute("aria-current", "page");
      else btn.removeAttribute("aria-current");
    });
    $("#page-title").textContent = PAGES[page].title;
    $("#page-sub").textContent = PAGES[page].sub;
    if (updateHash !== false) location.hash = page;
    view.innerHTML = loadingBlock();
    renderPage(page);
  }

  function bind() {
    $$(".nav-item").forEach(function (btn) {
      btn.addEventListener("click", function () { navigate(btn.getAttribute("data-page")); });
    });

    $("#stop-btn").addEventListener("click", stopFlow);

    $("#theme-btn").addEventListener("click", function () {
      var order = ["auto", "light", "dark"];
      var next = order[(order.indexOf(state.theme) + 1) % order.length];
      applyTheme(next);
    });

    $("#auth-form").addEventListener("submit", function (event) {
      event.preventDefault();
      var token = $("#auth-token").value.trim();
      if (!token) return;
      state.token = token;
      state.authOpen = false;
      localStorage.setItem("mock_token", token);
      $("#auth").hidden = true;
      refreshStatus();
      renderPage(state.page);
    });

    window.addEventListener("hashchange", function () {
      var page = (location.hash || "#overview").slice(1);
      if (page !== state.page) navigate(page, false);
    });
  }

  async function boot() {
    var urlTheme = new URLSearchParams(location.search).get("theme");
    if (urlTheme === "light" || urlTheme === "dark") state.theme = urlTheme;
    applyTheme(state.theme);
    bind();
    try { await ensureConfig(false); } catch (err) { /* 401 时由 openAuth 接管 */ }
    await refreshStatus();
    var initial = (location.hash || "#overview").slice(1);
    navigate(PAGES[initial] ? initial : "overview", false);

    setInterval(refreshStatus, 1000);
    setInterval(function () {
      if (state.page === "logs" && !state.authOpen) loadLogs(true);
    }, 2500);
  }

  boot();
})();
