package auth

import (
	"testing"
	"time"
)

func TestRotationGrace(t *testing.T) {
	now := time.Unix(0, 0)
	o := NewRouteObfuscator(true)
	o.now = func() time.Time { return now }

	old := o.CurrentMap()["kubeconfig"]
	now = now.Add(rotation)
	action, name, current, ok := o.Resolve(old + "/example")
	if !ok || action != "kubeconfig" || name != "example" || current[action] == old {
		t.Fatalf("previous route did not refresh: %q %q %v %t", action, name, current, ok)
	}
	now = now.Add(rotation)
	if _, _, _, ok := o.Resolve(old); ok {
		t.Fatal("expired route is still valid")
	}
}
