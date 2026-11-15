/* Lightweight i18n for the embedded, zero-build control UI. */
(function (global) {
  "use strict";

  var COOKIE = "mock_lang";
  var catalogs = {
    en: global.MOCK_LOCALE_EN || {},
    zh: global.MOCK_LOCALE_ZH || {}
  };
  var locale = detectLocale();

  function readCookie(name) {
    var match = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
    return match ? decodeURIComponent(match[1]) : "";
  }

  function queryLocale() {
    try {
      var value = new URLSearchParams(global.location.search).get("lang");
      return value === "en" || value === "zh" ? value : "";
    } catch (err) {
      return "";
    }
  }

  function detectLocale() {
    var fromQuery = queryLocale();
    if (fromQuery) return fromQuery;
    var saved = readCookie(COOKIE);
    if (saved === "en" || saved === "zh") return saved;
    var langs = navigator.languages && navigator.languages.length
      ? navigator.languages
      : [navigator.language || "en"];
    return String(langs[0] || "en").toLowerCase().indexOf("zh") === 0 ? "zh" : "en";
  }

  function interpolate(text, params) {
    if (!params) return text;
    return String(text).replace(/\{(\w+)\}/g, function (_, key) {
      return params[key] === undefined ? "{" + key + "}" : String(params[key]);
    });
  }

  function t(key, params) {
    var catalog = catalogs[locale] || catalogs.en;
    var text = catalog[key];
    if (text === undefined) text = catalogs.en[key];
    if (text === undefined) text = key;
    return interpolate(text, params);
  }

  function apiError(code, status) {
    if (code) {
      var key = "api.error." + code;
      if (catalogs[locale] && catalogs[locale][key] !== undefined) return t(key);
      if (catalogs.en[key] !== undefined) return t(key);
    }
    return t("error.requestFailed", { status: status });
  }

  function localeCode() {
    return locale === "zh" ? "zh-CN" : "en-US";
  }

  function formatNumber(value) {
    return Number(value || 0).toLocaleString(localeCode());
  }

  function formatDateTime(value) {
    if (!value) return t("common.none");
    var d = new Date(value);
    if (isNaN(d.getTime()) || d.getFullYear() < 2000) return t("common.none");
    return new Intl.DateTimeFormat(localeCode(), {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false
    }).format(d);
  }

  function formatClock(value) {
    if (!value) return "";
    var d = new Date(value);
    if (isNaN(d.getTime())) return "";
    return new Intl.DateTimeFormat(localeCode(), {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false
    }).format(d);
  }

  function setLocale(next) {
    if (next !== "en" && next !== "zh") return;
    if (next === locale) return;
    document.cookie = COOKIE + "=" + next + "; Path=/; Max-Age=31536000; SameSite=Lax";
    global.location.reload();
  }

  function applyStatic(root) {
    root = root || document;
    Array.prototype.forEach.call(root.querySelectorAll("[data-i18n]"), function (el) {
      el.textContent = t(el.getAttribute("data-i18n"));
    });
    Array.prototype.forEach.call(root.querySelectorAll("[data-i18n-aria-label]"), function (el) {
      el.setAttribute("aria-label", t(el.getAttribute("data-i18n-aria-label")));
    });
    Array.prototype.forEach.call(root.querySelectorAll("[data-i18n-placeholder]"), function (el) {
      el.setAttribute("placeholder", t(el.getAttribute("data-i18n-placeholder")));
    });
    Array.prototype.forEach.call(root.querySelectorAll("[data-lang]"), function (el) {
      var active = el.getAttribute("data-lang") === locale;
      el.classList.toggle("is-active", active);
      el.setAttribute("aria-pressed", active ? "true" : "false");
    });
  }

  function bindSwitcher() {
    Array.prototype.forEach.call(document.querySelectorAll("[data-lang]"), function (el) {
      el.addEventListener("click", function () {
        setLocale(el.getAttribute("data-lang"));
      });
    });
  }

  function init() {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    document.title = t("app.title");
    applyStatic(document);
    bindSwitcher();
  }

  global.MockI18n = {
    t: t,
    apiError: apiError,
    getLocale: function () { return locale; },
    localeCode: localeCode,
    setLocale: setLocale,
    formatNumber: formatNumber,
    formatDateTime: formatDateTime,
    formatClock: formatClock,
    applyStatic: applyStatic
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})(window);
