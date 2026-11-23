package handlers

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

//go:embed assets
var assets embed.FS

var staticFiles = http.FileServerFS(assets)

func (s *server) serveIndex(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	routes, _ := json.Marshal(s.routes.CurrentMap())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Routes-Map", string(routes))
	if r.Method != http.MethodHead {
		_, _ = w.Write(bytes.Replace(indexHTML, []byte("{{ROUTES_MAP}}"), routes, 1))
	}
}
