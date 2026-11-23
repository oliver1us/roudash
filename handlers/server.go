package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"roudash/auth"
	"roudash/k8s"
	"roudash/persist"
)

type server struct {
	manager *k8s.Manager
	store   *persist.Store
	routes  *auth.RouteObfuscator
	// Serialize changes involving both memory and disk.
	mutation sync.Mutex
}

func New(manager *k8s.Manager, store *persist.Store, routes *auth.RouteObfuscator) http.Handler {
	return &server{manager: manager, store: store, routes: routes}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.ico" {
		if !allow(w, r, http.MethodGet, http.MethodHead) {
			return
		}
		if r.URL.Path == "/favicon.ico" {
			r = r.Clone(r.Context())
			r.URL.Path = "/assets/favicon.ico"
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		staticFiles.ServeHTTP(w, r)
		return
	}
	switch r.URL.Path {
	case "/", "/index.html":
		s.serveIndex(w, r)
		return
	case "/healthz":
		if allow(w, r, http.MethodGet, http.MethodHead) {
			w.WriteHeader(http.StatusOK)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	action, name, current, ok := s.routes.Resolve(r.URL.Path)
	if !ok {
		w.Header().Set("X-Routes-Expired", "1")
		apiError(w, http.StatusUnauthorized, "unauthorized: invalid or expired obfuscated route")
		return
	}
	routesJSON, _ := json.Marshal(current)
	w.Header().Set("X-Routes-Map", string(routesJSON))
	switch action {
	case "status":
		s.status(w, r)
	case "contexts":
		s.contexts(w, r)
	case "context":
		s.switchContext(w, r)
	case "kubeconfig":
		if name == "" {
			s.upload(w, r)
		} else {
			s.deleteKubeconfig(w, r, name)
		}
	case "incluster":
		s.useInCluster(w, r)
	case "ingresses":
		s.ingresses(w, r)
	}
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodGet) {
		return
	}
	view := s.manager.Snapshot()
	state := "disconnected"
	if view.Connected {
		state = "connected"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success", "state": state, "activeContext": view.ActiveContext,
		"host": view.Host, "contexts": view.Contexts, "kubeconfigs": view.Kubeconfigs,
		"inCluster": view.InCluster, "persistMode": s.store.IsEnabled(),
	})
}

func (s *server) contexts(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodGet) {
		return
	}
	view := s.manager.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success", "activeContext": view.ActiveContext,
		"contexts": view.Contexts, "kubeconfigs": view.Kubeconfigs,
	})
}

func (s *server) ingresses(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodGet) {
		return
	}
	ingresses, err := s.manager.ListIngresses(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "data": ingresses})
}

func allow(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	apiError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

func apiError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"status": "error", "message": message})
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON: %v", err)
	}
}
