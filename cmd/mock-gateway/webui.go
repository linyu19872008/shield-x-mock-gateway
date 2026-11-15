package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web
var webAssets embed.FS

func newUIHandler() (http.Handler, error) {
	sub, err := fs.Sub(webAssets, "web")
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Cache-Control", "no-store")
		} else if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			// The embedded UI has no build-time asset hashing; avoid stale i18n bundles.
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	}), nil
}

// newServeHandler 组合控制 API 与内嵌 UI；静态资源不要求 Token，API 由 newControlMux 保护。
func newServeHandler(c *Controller, uiToken string) (http.Handler, error) {
	ui, err := newUIHandler()
	if err != nil {
		return nil, err
	}
	api := newControlMux(c, uiToken)
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/healthz", api)
	mux.Handle("/", ui)
	return mux, nil
}
