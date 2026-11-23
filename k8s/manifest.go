package k8s

import (
	"context"
	"encoding/json"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
)

// GetManifest reads one of the resource types referenced by the topology.
func (m *Manager) GetManifest(ctx context.Context, kind, namespace, name string) (json.RawMessage, error) {
	if len(validation.IsDNS1123Subdomain(name)) != 0 {
		return nil, apierrors.NewBadRequest("invalid resource name")
	}
	if kind != "namespace" && len(validation.IsDNS1123Label(namespace)) != 0 {
		return nil, apierrors.NewBadRequest("invalid namespace")
	}
	var path []string
	switch kind {
	case "namespace":
		path = []string{"/api/v1/namespaces", name}
	case "service":
		path = []string{"/api/v1/namespaces", namespace, "services", name}
	case "plugin":
		path = []string{"/apis/configuration.konghq.com/v1/namespaces", namespace, "kongplugins", name}
	default:
		return nil, apierrors.NewBadRequest("unsupported resource type")
	}
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return nil, apierrors.NewServiceUnavailable("no active Kubernetes connection")
	}
	get := func(path ...string) ([]byte, error) {
		return client.RESTClient().Get().AbsPath(path...).SetHeader("Accept", "application/json").DoRaw(ctx)
	}
	data, err := get(path...)
	if kind == "plugin" && apierrors.IsNotFound(err) {
		data, err = get("/apis/configuration.konghq.com/v1/kongclusterplugins", name)
	}
	return data, err
}
