package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"roudash/k8s"
)

type Store struct {
	dir string
}

type config struct {
	ActiveContext string `json:"activeContext,omitempty"`
}

func NewStore(dir string) (*Store, error) {
	s := &Store{dir: dir}
	if s.IsEnabled() {
		if err := os.MkdirAll(s.kubeconfigDir(), 0o700); err != nil {
			return nil, fmt.Errorf("create persisted kubeconfig directory: %w", err)
		}
	}
	return s, nil
}

func (s *Store) IsEnabled() bool {
	return s.dir != ""
}

func (s *Store) SaveKubeconfig(name string, data []byte) error {
	if !s.IsEnabled() {
		return nil
	}
	if !k8s.ValidName(name) {
		return errors.New("invalid kubeconfig name")
	}
	return atomicWrite(s.kubeconfigDir(), name+".yaml", data)
}

func (s *Store) DeleteKubeconfig(name string) error {
	if !s.IsEnabled() {
		return nil
	}
	if !k8s.ValidName(name) {
		return errors.New("invalid kubeconfig name")
	}
	err := os.Remove(filepath.Join(s.kubeconfigDir(), name+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) SaveConfig(activeContext string) error {
	if !s.IsEnabled() {
		return nil
	}
	data, err := json.MarshalIndent(config{ActiveContext: activeContext}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.dir, "config.json", data)
}

func (s *Store) Restore(manager *k8s.Manager) error {
	if !s.IsEnabled() {
		return nil
	}
	entries, err := os.ReadDir(s.kubeconfigDir())
	if err != nil {
		return err
	}
	var problems []error
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".yaml")
		if !ok || !entry.Type().IsRegular() || !k8s.ValidName(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.kubeconfigDir(), entry.Name()))
		if err == nil {
			err = manager.LoadFromBytes(name, data)
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("restore kubeconfig %q: %w", name, err))
		}
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "config.json"))
	if err == nil {
		var saved config
		if err = json.Unmarshal(data, &saved); err == nil && saved.ActiveContext != "" {
			err = manager.SwitchContext(saved.ActiveContext)
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		problems = append(problems, fmt.Errorf("restore active context: %w", err))
	}
	return errors.Join(problems...)
}

func (s *Store) kubeconfigDir() string {
	return filepath.Join(s.dir, "kubeconfigs")
}

func atomicWrite(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, ".roudash-*") // Creates a 0600 file.
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}
