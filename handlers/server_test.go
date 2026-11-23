package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"roudash/auth"
	"roudash/handlers"
	"roudash/k8s"
	"roudash/persist"
)

func TestWorkspaceLifecycle(t *testing.T) {
	fakeKube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/apis/networking.k8s.io/v1/ingresses" {
			t.Errorf("unexpected Kubernetes request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[{
			"metadata":{"name":"web","namespace":"demo","creationTimestamp":"2026-06-26T08:09:10Z","annotations":{"konghq.com/plugins":" auth, ,cors "}},
			"spec":{"ingressClassName":"kong","rules":[{"host":"app.example.invalid","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"web","port":{"number":8080}}}}]}}]},
			"status":{"loadBalancer":{"ingress":[{"hostname":"lb.example.invalid"}]}}
		}]}`)
	}))
	defer fakeKube.Close()

	dir := t.TempDir()
	store, err := persist.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	manager := k8s.NewManager()
	s := handlers.New(manager, store, auth.NewRouteObfuscator(false))
	request(t, s, "GET", "/healthz", "", nil, 200)
	status := request(t, s, "GET", "/status", "", nil, 200)
	contains(t, status.Body.String(), `"state":"disconnected"`, `"persistMode":true`, `"kubeconfigs":[]`)
	request(t, s, "GET", "/ingresses", "", nil, 500)

	cfg := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: fixture
  cluster:
    server: %s
users:
- name: fixture
  user: {}
contexts:
- name: first
  context: {cluster: fixture, user: fixture}
- name: second
  context: {cluster: fixture, user: fixture}
current-context: first
`, fakeKube.URL)
	t.Run("reject unsafe or empty entries", func(t *testing.T) {
		ct, body := multipartKubeconfig(t, "bad", "false", strings.Replace(cfg, "user: {}", "user: {tokenFile: /some/local/file}", 1))
		r := request(t, s, "POST", "/kubeconfig", ct, body, 422)
		contains(t, r.Body.String(), "use embedded credentials")
		ct, body = multipartKubeconfig(t, "empty", "false", "apiVersion: v1\nkind: Config\n")
		request(t, s, "POST", "/kubeconfig", ct, body, 422)
		contains(t, request(t, s, "GET", "/contexts", "", nil, 200).Body.String(), `"kubeconfigs":[]`)
	})

	ct, body := multipartKubeconfig(t, "Fixture Space", "true", cfg)
	r := request(t, s, "POST", "/kubeconfig", ct, body, 200)
	contains(t, r.Body.String(), `"saved":true`, `"Fixture Space:first"`, `"Fixture Space:second"`)

	path := filepath.Join(dir, "kubeconfigs", "Fixture Space.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("kubeconfig mode = %o; want 600", info.Mode().Perm())
	}
	r = request(t, s, "POST", "/context", "application/json", strings.NewReader(`{"context":"Fixture Space:second"}`), 200)
	contains(t, r.Body.String(), `"activeContext":"Fixture Space:second"`)
	r = request(t, s, "GET", "/ingresses", "", nil, 200)
	var result struct {
		Data []k8s.Ingress
	}
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 || len(result.Data[0].Paths) != 1 {
		t.Fatalf("unexpected ingress response: %v", result)
	}
	i := result.Data[0]
	if i.Name != "web" || i.Namespace != "demo" || i.IngressClass != "kong" ||
		strings.Join(i.Plugins, ",") != "auth,cors" || i.LoadBalancer != "lb.example.invalid" ||
		i.CreationTime != "2026-06-26 08:09:10 +0000 UTC" ||
		i.Paths[0] != (k8s.Path{Host: "app.example.invalid", Path: "/", PathType: "Prefix", ServiceName: "web", ServicePort: "8080"}) {
		t.Fatalf("unexpected ingress: %+v", i)
	}

	restored := k8s.NewManager()
	if err := store.Restore(restored); err != nil {
		t.Fatal(err)
	}
	if got := restored.Snapshot().ActiveContext; got != "Fixture Space:second" {
		t.Fatalf("restored active context = %q", got)
	}
	s = handlers.New(restored, store, auth.NewRouteObfuscator(false))
	request(t, s, "DELETE", "/kubeconfig/Fixture%20Space", "", nil, 200)
	contains(t, request(t, s, "GET", "/status", "", nil, 200).Body.String(), `"state":"disconnected"`)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("saved kubeconfig not removed: %v", err)
	}
}

func TestEmbeddedPageWithObfuscatedRoutes(t *testing.T) {
	store, err := persist.NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := handlers.New(k8s.NewManager(), store, auth.NewRouteObfuscator(true))
	index := request(t, s, "GET", "/", "", nil, 200)
	match := regexp.MustCompile(`let ROUTES_MAP = (\{[^\n]+\});`).FindSubmatch(index.Body.Bytes())
	if len(match) != 2 || strings.Contains(index.Body.String(), "{{ROUTES_MAP}}") {
		t.Fatal("embedded page did not receive its routes")
	}
	var routes map[string]string
	if err := json.Unmarshal(match[1], &routes); err != nil || len(routes) != 6 {
		t.Fatalf("invalid routes: %v (%v)", routes, err)
	}
	request(t, s, "GET", "/status", "", nil, 401)
	r := request(t, s, "GET", routes["status"], "", nil, 200)
	if r.Header().Get("X-Routes-Map") == "" {
		t.Fatal("missing X-Routes-Map")
	}
}

func multipartKubeconfig(t *testing.T, name, save, config string) (string, io.Reader) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", name); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("save", save); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "fixture.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, config); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return writer.FormDataContentType(), &body
}

func request(t *testing.T, h http.Handler, method, path, content string, body io.Reader, expected int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, body)
	r.Header.Set("Content-Type", content)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, r)
	if res.Code != expected {
		t.Fatalf("%s %s = %d; want %d: %s", method, path, res.Code, expected, res.Body)
	}
	return res
}

func contains(t *testing.T, got string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(got, value) {
			t.Errorf("%q does not contain %q", got, value)
		}
	}
}
