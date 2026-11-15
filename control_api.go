package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// newControlMux 构造控制台 API。uiToken 非空时要求 X-Mock-Token 或 Bearer。
func newControlMux(c *Controller, uiToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, c.Status())
	})
	mux.HandleFunc("POST /api/start", func(w http.ResponseWriter, r *http.Request) {
		handleStart(c, w, r)
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := c.Stop(); err != nil {
			if errors.Is(err, ErrNotRunning) {
				writeAPIError(w, http.StatusConflict, "NOT_RUNNING", err.Error())
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "STOP_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) {
		limit := 200
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				writeAPIError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit 必须是正整数")
				return
			}
			if n > 1000 {
				n = 1000
			}
			limit = n
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": c.Logs(limit)})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, configViewOf(c.cfg))
	})
	mux.HandleFunc("GET /api/policies", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, c.Policies())
	})
	mux.HandleFunc("GET /api/templates", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": c.Templates()})
	})
	mux.HandleFunc("POST /api/fault", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Down bool `json:"down"`
		}
		if err := decodeBody(r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		c.SetDown(req.Down)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "down": req.Down})
	})

	return withControlAuth(mux, uiToken)
}

type startRequest struct {
	Mode     string `json:"mode"`
	Rate     int    `json:"rate"`
	Duration string `json:"duration"`
}

func handleStart(c *Controller, w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := decodeBody(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	var duration time.Duration
	if req.Duration != "" {
		d, err := time.ParseDuration(req.Duration)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_DURATION", "duration 例如 30s / 5m")
			return
		}
		duration = d
	}

	if err := c.Start(req.Mode, req.Rate, duration); err != nil {
		switch {
		case errors.Is(err, ErrAlreadyRunning):
			writeAPIError(w, http.StatusConflict, "ALREADY_RUNNING", err.Error())
		case errors.Is(err, ErrInvalidMode):
			writeAPIError(w, http.StatusBadRequest, "INVALID_MODE", err.Error())
		default:
			writeAPIError(w, http.StatusBadRequest, "START_FAILED", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusAccepted, c.Status())
}

func decodeBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

type configView struct {
	BaseURL        string       `json:"base_url"`
	GatewayID      string       `json:"gateway_id"`
	AgentVersion   string       `json:"agent_version"`
	TokenSet       bool         `json:"token_set"`
	PollInterval   string       `json:"poll_interval"`
	ReportInterval string       `json:"report_interval"`
	ReportFormat   string       `json:"report_format"`
	Rate           int          `json:"rate"`
	Workers        int          `json:"workers"`
	SpoolDir       string       `json:"spool_dir"`
	FetchTemplates bool         `json:"fetch_on_missing"`
	Capabilities   Capabilities `json:"capabilities"`
	Fault          faultView    `json:"fault"`
}

type faultView struct {
	ShieldXDown        bool   `json:"shieldx_down"`
	Latency            string `json:"latency"`
	DropPercent        int    `json:"drop_percent"`
	HTTPStatusOverride int    `json:"http_status_override"`
}

func configViewOf(cfg *Config) configView {
	return configView{
		BaseURL:        cfg.ShieldX.BaseURL,
		GatewayID:      cfg.ShieldX.GatewayID,
		AgentVersion:   cfg.ShieldX.AgentVersion,
		TokenSet:       cfg.ShieldX.Token != "",
		PollInterval:   cfg.Poll.Interval.D().String(),
		ReportInterval: cfg.Report.Interval.D().String(),
		ReportFormat:   cfg.Report.Format,
		Rate:           cfg.Generate.Rate,
		Workers:        cfg.Generate.Workers,
		SpoolDir:       cfg.Report.SpoolDir,
		FetchTemplates: cfg.Template.FetchOnMissing,
		Capabilities:   cfg.ShieldX.Capabilities,
		Fault: faultView{
			ShieldXDown:        cfg.Fault.ShieldXDown,
			Latency:            cfg.Fault.Latency.D().String(),
			DropPercent:        cfg.Fault.DropPercent,
			HTTPStatusOverride: cfg.Fault.HTTPStatusOverride,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"code": code, "message": msg},
	})
}

func withControlAuth(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-Mock-Token")
		if got == "" {
			got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "缺少或错误的控制台 Token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
