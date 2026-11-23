package k8s

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	networking "k8s.io/client-go/kubernetes/typed/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubeconfig "k8s.io/client-go/tools/clientcmd/api"
)

type Manager struct {
	mu      sync.RWMutex
	configs map[string]*kubeconfig.Config
	active  string
	host    string
	client  networking.NetworkingV1Interface
	cluster bool
}

type Snapshot struct {
	ActiveContext string
	Host          string
	Contexts      []string
	Kubeconfigs   []string
	Connected     bool
	InCluster     bool
}

func NewManager() *Manager {
	return &Manager{configs: make(map[string]*kubeconfig.Config)}
}

func ValidName(name string) bool {
	return name != "" && len(name) <= 200 && name != "." && name != ".." &&
		!strings.ContainsAny(name, `/\:`) && !strings.ContainsFunc(name, unicode.IsControl)
}

func (m *Manager) LoadFromBytes(name string, data []byte) error {
	if !ValidName(name) {
		return errors.New("invalid kubeconfig name")
	}
	cfg, err := clientcmd.Load(data)
	if err != nil {
		return err
	}
	// Uploaded kubeconfigs are data, not permission to run programs or read
	// arbitrary files on the machine hosting RouDash.
	for _, user := range cfg.AuthInfos {
		if user == nil {
			return errors.New("invalid kubeconfig user")
		}
		if user.Exec != nil || user.AuthProvider != nil || user.TokenFile != "" ||
			user.ClientCertificate != "" || user.ClientKey != "" {
			return errors.New("use embedded credentials; exec, auth-provider and local credential files are not supported")
		}
	}
	for _, cluster := range cfg.Clusters {
		if cluster == nil || cluster.CertificateAuthority != "" {
			return errors.New("use certificate-authority-data instead of a local CA file")
		}
	}

	contexts := make([]string, 0, len(cfg.Contexts))
	for contextName, ctx := range cfg.Contexts {
		if contextName == "" || ctx == nil {
			return errors.New("invalid kubeconfig context")
		}
		contexts = append(contexts, contextName)
	}
	if len(contexts) == 0 {
		return fmt.Errorf("kubeconfig %q has no contexts", name)
	}
	preferred := cfg.CurrentContext
	if preferred == "" {
		sort.Strings(contexts)
		preferred = contexts[0]
	} else if _, ok := cfg.Contexts[preferred]; !ok {
		return fmt.Errorf("context %q not found in kubeconfig %q", preferred, name)
	}
	config, client, err := makeClient(cfg, preferred)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[name] = cfg
	m.setActive(name+":"+preferred, config.Host, client, false)
	return nil
}

func (m *Manager) SwitchContext(requested string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name, contextName, scoped := strings.Cut(requested, ":"); scoped && m.configs[name] != nil {
		return m.switchContext(name, contextName)
	}
	var matches []string
	for name, cfg := range m.configs {
		if _, ok := cfg.Contexts[requested]; ok {
			matches = append(matches, name)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("context %q not found in any loaded kubeconfig", requested)
	case 1:
		return m.switchContext(matches[0], requested)
	default:
		return fmt.Errorf("context %q is ambiguous; use kubeconfig:context", requested)
	}
}

func (m *Manager) switchContext(name, contextName string) error {
	cfg := m.configs[name]
	if _, ok := cfg.Contexts[contextName]; !ok {
		return fmt.Errorf("context %q not found in kubeconfig %q", contextName, name)
	}
	config, client, err := makeClient(cfg, contextName)
	if err != nil {
		return err
	}
	m.setActive(name+":"+contextName, config.Host, client, false)
	return nil
}

func (m *Manager) LoadInCluster() error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("in-cluster config failed: %w", err)
	}
	config.Timeout = 15 * time.Second
	client, err := networking.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("in-cluster client: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setActive("in-cluster", config.Host, client, true)
	return nil
}

func (m *Manager) RemoveKubeconfig(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.configs, name)
	if strings.HasPrefix(m.active, name+":") {
		m.setActive("", "", nil, false)
	}
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Snapshot{
		ActiveContext: m.active,
		Host:          m.host,
		Kubeconfigs:   make([]string, 0, len(m.configs)),
		Connected:     m.client != nil,
		InCluster:     m.cluster,
	}
	for name, cfg := range m.configs {
		out.Kubeconfigs = append(out.Kubeconfigs, name)
		for contextName := range cfg.Contexts {
			out.Contexts = append(out.Contexts, name+":"+contextName)
		}
	}
	sort.Strings(out.Kubeconfigs)
	sort.Strings(out.Contexts)
	return out
}

func (m *Manager) ListIngresses(ctx context.Context) ([]Ingress, error) {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return nil, errors.New("no active Kubernetes connection — please upload a kubeconfig")
	}
	list, err := client.Ingresses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return mapIngresses(list.Items), nil
}

func makeClient(cfg *kubeconfig.Config, contextName string) (*rest.Config, networking.NetworkingV1Interface, error) {
	config, err := clientcmd.NewNonInteractiveClientConfig(*cfg, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, nil, err
	}
	config.Timeout = 15 * time.Second
	client, err := networking.NewForConfig(config)
	return config, client, err
}

func (m *Manager) setActive(contextName, host string, client networking.NetworkingV1Interface, inCluster bool) {
	m.active, m.host, m.client, m.cluster = contextName, host, client, inCluster
}
