(function () {
  "use strict";

  var tr = MockI18n.t;
  var PAGES = {
    overview: { titleKey: "page.overview.title", subKey: "page.overview.subtitle" },
    connection: { titleKey: "page.connection.title", subKey: "page.connection.subtitle" },
    policies: { titleKey: "page.policies.title", subKey: "page.policies.subtitle" },
    traffic: { titleKey: "page.traffic.title", subKey: "page.traffic.subtitle" },
    fault: { titleKey: "page.fault.title", subKey: "page.fault.subtitle" },
    spool: { titleKey: "page.spool.title", subKey: "page.spool.subtitle" },
    logs: { titleKey: "page.logs.title", subKey: "page.logs.subtitle" }
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
    return Number.isFinite(n) ? MockI18n.formatNumber(n) : "0";
  }

  function fmtBytes(n) {
    n = Number(n || 0);
    if (n < 1024) return n + " B";
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KiB";
    return (n / 1024 / 1024).toFixed(1) + " MiB";
  }

  function fmtTime(value) {
    return MockI18n.formatDateTime(value);
  }

  function fmtClock(value) {
    return MockI18n.formatClock(value);
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
      throw new Error(tr("error.consoleConnection", { error: err.message }));
    }

    if (res.status === 401) {
      openAuth();
      throw new Error(tr("api.error.UNAUTHORIZED"));
    }

    var text = await res.text();
    var data = null;
    if (text) {
      try { data = JSON.parse(text); } catch (err) { data = null; }
    }
    if (!res.ok) {
      var code = data && data.error ? data.error.code : "";
      throw new Error(MockI18n.apiError(code, res.status));
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
    $("#theme-btn").textContent = tr("theme.label", { theme: tr("theme." + theme) });
  }

  function renderChrome() {
    var pill = $("#run-pill");
    var stopBtn = $("#stop-btn");
    if (!state.online) {
      pill.className = "pill bad";
      pill.innerHTML = '<span class="dot"></span>' + esc(tr("status.offline"));
      stopBtn.disabled = true;
      return;
    }
    var s = state.status;
    if (!s) {
      pill.className = "pill";
      pill.innerHTML = '<span class="dot"></span>' + esc(tr("status.connecting"));
      stopBtn.disabled = true;
      return;
    }
    if (s.running) {
      pill.className = "pill ok";
      pill.innerHTML = '<span class="dot"></span>' + esc(tr("status.running", { mode: s.mode || "run" }));
      stopBtn.disabled = false;
    } else {
      pill.className = "pill";
      pill.innerHTML = '<span class="dot"></span>' + esc(tr("status.idle"));
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
      if (err.message !== tr("api.error.UNAUTHORIZED")) {
        state.online = false;
        renderChrome();
      }
    }
  }

  function loadingBlock() {
    return '<div class="loading">' + esc(tr("common.loading")) + "</div>";
  }

  function errorBlock(message) {
    return '<div class="card"><div class="callout error"><strong>' + esc(tr("error.loadFailed", { error: message })) + "</strong></div></div>";
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
    var modeLabel = tr(s.mode === "loadtest" ? "overview.mode.loadtest" : "overview.mode.run");
    var gateway = state.config ? state.config.gateway_id : "";

    var cards = [
      ["overview.stat.generated", num(s.generated), "overview.stat.generated.foot"],
      ["overview.stat.enqueued", num(s.enqueued), "overview.stat.enqueued.foot"],
      ["overview.stat.sent", num(s.sent), "overview.stat.sent.foot"],
      ["overview.stat.accepted", num(s.accepted), "overview.stat.accepted.foot"],
      ["overview.stat.rejected", num(s.rejected), "overview.stat.rejected.foot"],
      ["overview.stat.dropped", num(s.dropped), "overview.stat.dropped.foot"],
      ["overview.stat.queue", num(s.queue_depth), "overview.stat.queue.foot"],
      ["overview.stat.spool", num(s.spool_events), "overview.stat.spool.foot"]
    ].map(function (item) {
      return statCard(tr(item[0] + ".label"), item[1], tr(item[2]));
    }).join("");

    var pollTotal = Number(s.poll_success || 0) + Number(s.poll_not_modified || 0);
    var lastError = s.last_error
      ? '<div class="callout error"><strong>' + esc(tr("overview.lastError")) + '</strong>' + esc(s.last_error) + "</div>"
      : '<div class="callout">' + esc(tr("overview.noError")) + "</div>";

    view.innerHTML =
      '<section class="card hero">' +
        '<div>' +
          '<span class="eyebrow">' + esc(tr(running ? "overview.eyebrow.running" : "overview.eyebrow.idle")) + "</span>" +
          "<h2>" + esc(running ? tr("overview.heading.running", { mode: modeLabel }) : tr("overview.heading.ready")) + "</h2>" +
          "<p>" + esc(running
            ? tr("overview.running.description", { uptime: fmtUptime(s.uptime_seconds), gateway: gateway || tr("overview.unknown") })
            : tr("overview.idle.description")) +
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
          '<div class="card-head"><div><h2>' + esc(tr("overview.poll.title")) + "</h2><p>" + esc(tr("overview.poll.subtitle")) + "</p></div></div>" +
          '<dl class="kv">' +
            "<dt>" + esc(tr("overview.poll.success")) + "</dt><dd>" + num(s.poll_success) + "</dd>" +
            "<dt>" + esc(tr("overview.poll.notModified")) + "</dt><dd>" + num(s.poll_not_modified) + "（" + esc(pct(s.poll_not_modified, pollTotal)) + "）</dd>" +
            "<dt>" + esc(tr("overview.poll.failed")) + "</dt><dd>" + num(s.poll_failures) + "</dd>" +
            "<dt>" + esc(tr("overview.poll.revision")) + "</dt><dd>" + num(s.revision) + "</dd>" +
            "<dt>" + esc(tr("overview.poll.counts")) + "</dt><dd>" + num(s.policies) + " / " + num(s.templates) + "</dd>" +
          "</dl>" +
        "</section>" +
        '<section class="card">' +
          '<div class="card-head"><div><h2>' + esc(tr("overview.errors.title")) + "</h2><p>" + esc(tr("overview.errors.subtitle")) + "</p></div></div>" +
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
        '<div class="card-head"><div><h2>' + esc(tr("connection.title")) + "</h2><p>" + esc(tr("connection.subtitle")) + "</p></div></div>" +
        '<dl class="kv">' +
          "<dt>" + esc(tr("connection.baseUrl")) + "</dt><dd>" + esc(cfg.base_url) + "</dd>" +
          "<dt>" + esc(tr("connection.gatewayId")) + "</dt><dd>" + esc(cfg.gateway_id) + "</dd>" +
          "<dt>" + esc(tr("connection.agentVersion")) + "</dt><dd>" + esc(cfg.agent_version) + "</dd>" +
          "<dt>" + esc(tr("connection.tenantToken")) + "</dt><dd>" + esc(tr(cfg.token_set ? "connection.tokenSet" : "connection.tokenMissing")) + "</dd>" +
          "<dt>" + esc(tr("connection.pollInterval")) + "</dt><dd>" + esc(cfg.poll_interval) + "</dd>" +
          "<dt>" + esc(tr("connection.reportInterval")) + "</dt><dd>" + esc(cfg.report_interval) + " · " + esc(cfg.report_format) + "</dd>" +
          "<dt>" + esc(tr("connection.spoolDir")) + "</dt><dd>" + esc(cfg.spool_dir || tr("connection.spoolDisabled")) + "</dd>" +
        "</dl>" +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("connection.capabilities.title")) + "</h2><p>" + esc(tr("connection.capabilities.subtitle")) + "</p></div></div>" +
        '<div class="chips" style="margin-bottom:12px">' + (actions || '<span class="muted small">' + esc(tr("connection.actionsEmpty")) + "</span>") + "</div>" +
        '<div class="chips" style="margin-bottom:12px">' + (fields || '<span class="muted small">' + esc(tr("connection.fieldsEmpty")) + "</span>") + "</div>" +
        '<dl class="kv">' +
          "<dt>" + esc(tr("connection.customTemplates")) + "</dt><dd>" + esc(tr(caps.block_templates ? "common.supported" : "common.unsupported")) + "</dd>" +
          "<dt>" + esc(tr("connection.templateSpecVersion")) + "</dt><dd>" + num(caps.template_spec_version) + "</dd>" +
          "<dt>" + esc(tr("connection.maxRules")) + "</dt><dd>" + num(caps.max_rules) + "</dd>" +
        "</dl>" +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("connection.fault.title")) + "</h2><p>" + esc(tr("connection.fault.subtitle")) + "</p></div></div>" +
        '<dl class="kv">' +
          "<dt>" + esc(tr("connection.fault.down")) + "</dt><dd>" + esc(tr(fault.shieldx_down ? "common.on" : "common.off")) + "</dd>" +
          "<dt>" + esc(tr("connection.fault.latency")) + "</dt><dd>" + esc(fault.latency || "0s") + "</dd>" +
          "<dt>" + esc(tr("connection.fault.drop")) + "</dt><dd>" + num(fault.drop_percent) + "%</dd>" +
          "<dt>" + esc(tr("connection.fault.status")) + "</dt><dd>" + esc(fault.http_status_override ? num(fault.http_status_override) : tr("connection.fault.statusOff")) + "</dd>" +
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
        '<div class="card-head"><div><h2>' + esc(tr("policies.title")) + "</h2><p>" +
          esc(tr("policies.subtitle", {
            revision: num(snap.revision),
            time: fmtTime(snap.fetched_at),
            server: snap.server || tr("common.none")
          })) + "</p></div>" +
          '<button id="refresh-policies" class="btn btn-ghost" type="button">' + esc(tr("policies.refresh")) + "</button></div>";

    var body = policies.length
      ? policies.map(function (p) {
          var action = (p.action && p.action.type) || "none";
          return '<details class="policy">' +
            "<summary>" +
              '<span class="prio">' + num(p.priority) + "</span>" +
              '<span class="policy-name">' + esc(p.name || tr("policies.unnamed")) + "</span>" +
              '<span class="policy-id">' + esc(p.id) + "</span>" +
              '<span class="spacer"></span>' +
              '<span class="badge' + policyActionClass(action) + '">' + esc(action) + "</span>" +
            "</summary>" +
            "<pre>" + esc(JSON.stringify({ match: p.match, action: p.action }, null, 2)) + "</pre>" +
          "</details>";
        }).join("")
      : emptyBlock(tr("policies.empty.title"), tr("policies.empty.body"));

    var tplBody = templates.length
      ? '<div class="tpl-grid">' + templates.map(function (tpl) {
          var vars = Object.keys(tpl.vars || {}).map(function (k) {
            return '<span class="chip">' + esc(k) + "</span>";
          }).join("");
          var headers = Object.keys(tpl.headers || {}).map(function (k) {
            return '<span class="chip">' + esc(k) + "</span>";
          }).join("");
          return '<article class="tpl-card">' +
            "<header><h3>" + esc(tpl.name || tpl.id) + "</h3>" +
              '<p class="meta">' + esc(tpl.id) + " · v" + num(tpl.version) + "</p>" +
              '<p class="meta">' + esc(tpl.content_type || "") + " · " + num(tpl.status_code) + "</p></header>" +
            '<pre class="tpl-preview">' + esc(tpl.content || "") + "</pre>" +
            "<footer>" + (vars || headers
              ? '<div class="chips">' + vars + headers + "</div>"
              : '<span class="muted small">' + esc(tr("policies.noVars")) + "</span>") + "</footer>" +
          "</article>";
        }).join("") + "</div>"
      : emptyBlock(tr("policies.templates.empty.title"), tr("policies.templates.empty.body"));

    view.innerHTML = header + body +
      '<div class="card-head" style="margin-top:16px"><div><h2>' + esc(tr("policies.templates.title")) +
      "</h2><p>" + esc(tr("policies.templates.subtitle")) + "</p></div></div>" +
      tplBody + "</section>";

    var btn = $("#refresh-policies");
    if (btn) {
      btn.addEventListener("click", async function () {
        btn.disabled = true;
        btn.textContent = tr("policies.refreshing");
        try {
          await loadPolicies(true);
          renderPolicies();
          toast(tr("policies.refreshed"));
        } catch (err) {
          toast(err.message, "error");
          btn.disabled = false;
          btn.textContent = tr("policies.refresh");
        }
      });
    }
  }

  function renderTraffic() {
    var cfg = state.config || {};
    var rate = Number(cfg.rate || 20);
    view.innerHTML =
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("traffic.title")) + "</h2><p>" + esc(tr("traffic.subtitle")) + "</p></div></div>" +
        '<p class="hint" style="margin:0 0 12px">' + esc(tr("traffic.hint")) + "</p>" +
        '<div class="form-grid">' +
          '<label class="field">' + esc(tr("traffic.mode")) + '<select id="traffic-mode">' +
            '<option value="run">' + esc(tr("traffic.mode.run")) + "</option>" +
            '<option value="loadtest">' + esc(tr("traffic.mode.loadtest")) + "</option>" +
          "</select></label>" +
          '<label class="field">' + esc(tr("traffic.rate")) + '<input id="traffic-rate" type="number" min="1" value="' + rate + '"></label>' +
          '<label class="field">' + esc(tr("traffic.duration")) + '<input id="traffic-duration" type="text" value="30s" disabled></label>' +
        "</div>" +
        '<div class="row-actions">' +
          '<button id="traffic-start" class="btn btn-primary" type="button">' + esc(tr("common.start")) + "</button>" +
          '<button id="traffic-stop" class="btn btn-ghost" type="button">' + esc(tr("common.stop")) + "</button>" +
          '<span class="hint">' + esc(tr("traffic.presets")) + "</span>" +
          '<button class="btn btn-ghost" type="button" data-rate="20">20</button>' +
          '<button class="btn btn-ghost" type="button" data-rate="200">200</button>' +
          '<button class="btn btn-ghost" type="button" data-rate="1000">1000</button>' +
          '<button class="btn btn-ghost" type="button" data-rate="3000">3000</button>' +
        "</div>" +
        '<div id="traffic-state" class="inline-status"></div>' +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("traffic.note.title")) + "</h2></div></div>" +
        '<div class="callout">' + esc(tr("traffic.note.body")) + "</div>" +
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
      ? tr("traffic.running", {
          mode: s.mode || "run",
          rate: num(s.rate),
          generated: num(s.generated),
          accepted: num(s.accepted),
          queue: num(s.queue_depth)
        })
      : tr("traffic.notRunning");
  }

  async function startFlow() {
    var modeEl = $("#traffic-mode");
    var rateEl = $("#traffic-rate");
    var durEl = $("#traffic-duration");
    if (!modeEl || !rateEl) return;

    var rate = Number(rateEl.value);
    if (!Number.isFinite(rate) || rate < 1) {
      toast(tr("traffic.invalidRate"), "error");
      return;
    }
    var mode = modeEl.value;
    var duration = mode === "loadtest" ? durEl.value.trim() : "";
    if (mode === "loadtest" && !duration) {
      toast(tr("traffic.durationRequired"), "error");
      return;
    }

    var btn = $("#traffic-start");
    btn.disabled = true;
    btn.textContent = tr("common.starting");
    try {
      await api("/api/start", {
        method: "POST",
        body: JSON.stringify({ mode: mode, rate: rate, duration: duration })
      });
      toast(tr("traffic.started", { mode: mode }));
      await refreshStatus();
    } catch (err) {
      toast(err.message, "error");
    } finally {
      btn.disabled = false;
      btn.textContent = tr("common.start");
    }
  }

  async function stopFlow() {
    try {
      await api("/api/stop", { method: "POST", body: "{}" });
      toast(tr("traffic.stopped"));
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
        '<div class="card-head"><div><h2>' + esc(tr("fault.title")) + "</h2><p>" + esc(tr("fault.subtitle")) + "</p></div></div>" +
        '<label class="switch">' +
          '<input id="fault-down" type="checkbox"' + (fault.shieldx_down ? " checked" : "") + ">" +
          '<span class="switch-track"></span>' +
          "<span>" + esc(tr("fault.toggle")) + "</span>" +
        "</label>" +
        '<div class="callout warn" style="margin-top:14px">' + esc(tr("fault.warning")) + "</div>" +
      "</section>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("fault.static.title")) + "</h2><p>" + esc(tr("fault.static.subtitle")) + "</p></div></div>" +
        '<dl class="kv">' +
          "<dt>" + esc(tr("fault.latency")) + "</dt><dd>" + esc(fault.latency || "0s") + "</dd>" +
          "<dt>" + esc(tr("fault.drop")) + "</dt><dd>" + num(fault.drop_percent) + "%</dd>" +
          "<dt>" + esc(tr("fault.status")) + "</dt><dd>" + esc(fault.http_status_override ? num(fault.http_status_override) : tr("fault.statusOff")) + "</dd>" +
        "</dl>" +
      "</section>";

    var input = $("#fault-down");
    input.addEventListener("change", async function () {
      var down = input.checked;
      input.disabled = true;
      try {
        await api("/api/fault", { method: "POST", body: JSON.stringify({ down: down }) });
        fault.shieldx_down = down;
        toast(tr(down ? "fault.enabled" : "fault.recovered"));
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
        statCard(tr("spool.pending.label"), num(s.spool_events), tr("spool.pending.foot")) +
        statCard(tr("spool.files.label"), num(s.spool_files), tr("spool.files.foot")) +
        statCard(tr("spool.size.label"), fmtBytes(s.spool_bytes), tr("spool.size.foot")) +
      "</div>" +
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("spool.title")) + "</h2><p>" + esc(tr("spool.subtitle")) + "</p></div></div>" +
        '<div class="callout">' +
          tr("spool.body", { dir: esc(cfg.spool_dir || tr("spool.dirDisabled")) }) +
        "</div>" +
        '<dl class="kv" style="margin-top:12px">' +
          "<dt>" + esc(tr("spool.queue")) + "</dt><dd>" + num(s.queue_depth) + "</dd>" +
          "<dt>" + esc(tr("spool.disk")) + "</dt><dd>" + num(s.spooled) + "</dd>" +
          "<dt>" + esc(tr("spool.replayed")) + "</dt><dd>" + num(s.replayed) + "</dd>" +
        "</dl>" +
      "</section>";
  }

  function renderLogs() {
    view.innerHTML =
      '<section class="card">' +
        '<div class="card-head"><div><h2>' + esc(tr("logs.title")) + "</h2><p>" + esc(tr("logs.subtitle")) + "</p></div></div>" +
        '<div class="toolbar">' +
          '<input id="log-filter" class="filter" placeholder="' + esc(tr("logs.filter")) + '" value="' + esc(state.logFilter) + '">' +
          '<button id="refresh-logs" class="btn btn-ghost" type="button">' + esc(tr("logs.refresh")) + "</button>" +
        "</div>" +
        '<div id="log-list"></div>' +
      "</section>" +
      '<details class="card">' +
        "<summary><strong>" + esc(tr("logs.config")) + '</strong> <span class="muted small">' + esc(tr("logs.expand")) + "</span></summary>" +
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
      box.innerHTML = emptyBlock(
        tr("logs.empty.filtered"),
        state.logFilter ? tr("logs.empty.hint") : tr("logs.empty.none")
      );
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
    $("#page-title").textContent = tr(PAGES[page].titleKey);
    $("#page-sub").textContent = tr(PAGES[page].subKey);
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
