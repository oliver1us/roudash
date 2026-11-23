package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"roudash/k8s"
)

const maxKubeconfig = 1 << 20

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxKubeconfig+(16<<10))
	if err := r.ParseMultipartForm(maxKubeconfig + (16 << 10)); err != nil {
		apiError(w, http.StatusRequestEntityTooLarge, "kubeconfig file too large (max 1 MiB)")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		apiError(w, http.StatusBadRequest, "missing 'file' field in form")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxKubeconfig+1))
	if err != nil {
		apiError(w, http.StatusBadRequest, "could not read kubeconfig")
		return
	}
	if len(data) > maxKubeconfig {
		apiError(w, http.StatusRequestEntityTooLarge, "kubeconfig file too large (max 1 MiB)")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}

	s.mutation.Lock()
	defer s.mutation.Unlock()
	if err := s.manager.LoadFromBytes(name, data); err != nil {
		apiError(w, http.StatusUnprocessableEntity, "invalid kubeconfig: "+err.Error())
		return
	}
	saved := false
	if s.store.IsEnabled() && r.FormValue("save") == "true" {
		if err := s.store.SaveKubeconfig(name, data); err != nil {
			log.Printf("[persist] save kubeconfig %q: %v", name, err)
		} else {
			saved = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success", "contexts": s.manager.Snapshot().Contexts, "name": name, "saved": saved,
	})
}

func (s *server) switchContext(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var req struct {
		Context string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Context) == "" {
		apiError(w, http.StatusBadRequest, `body must be {"context": "<name>"}`)
		return
	}

	s.mutation.Lock()
	defer s.mutation.Unlock()
	if err := s.manager.SwitchContext(req.Context); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	view := s.manager.Snapshot()
	s.saveActiveContext(view.ActiveContext)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "success", "activeContext": view.ActiveContext, "host": view.Host,
	})
}

func (s *server) useInCluster(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, http.MethodPost) {
		return
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	if err := s.manager.LoadInCluster(); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view := s.manager.Snapshot()
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "success", "activeContext": view.ActiveContext, "host": view.Host,
	})
}

func (s *server) deleteKubeconfig(w http.ResponseWriter, r *http.Request, name string) {
	if !allow(w, r, http.MethodDelete) {
		return
	}
	if !k8s.ValidName(name) {
		apiError(w, http.StatusBadRequest, "invalid kubeconfig name")
		return
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	if err := s.store.DeleteKubeconfig(name); err != nil {
		log.Printf("[persist] delete kubeconfig %q: %v", name, err)
		apiError(w, http.StatusInternalServerError, "could not delete saved kubeconfig")
		return
	}
	before := s.manager.Snapshot().ActiveContext
	s.manager.RemoveKubeconfig(name)
	if after := s.manager.Snapshot().ActiveContext; before != after {
		s.saveActiveContext(after)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "name": name})
}

func (s *server) saveActiveContext(contextName string) {
	if err := s.store.SaveConfig(contextName); err != nil {
		log.Printf("[persist] save active context: %v", err)
	}
}
