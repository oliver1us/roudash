package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const rotation = 30 * time.Second

var actions = [...]string{"ingresses", "manifest", "status", "contexts", "context", "kubeconfig", "incluster"}

// RouteObfuscator preserves the browser's routing protocol. It is not authentication:
// the current paths are included in the publicly served page.
type RouteObfuscator struct {
	enabled bool
	key     [32]byte
	now     func() time.Time
}

func NewRouteObfuscator(enabled bool) *RouteObfuscator {
	o := &RouteObfuscator{enabled: enabled, now: time.Now}
	if enabled {
		rand.Read(o.key[:])
	}
	return o
}

func (o *RouteObfuscator) CurrentMap() map[string]string {
	return o.routes(o.epoch())
}

func (o *RouteObfuscator) Resolve(path string) (action, name string, current map[string]string, ok bool) {
	epoch := o.epoch()
	current = o.routes(epoch)
	action, name, ok = resolve(path, current)
	if !ok && o.enabled {
		// Allow requests still using the map from the previous 30-second interval.
		action, name, ok = resolve(path, o.routes(epoch-1))
	}
	return
}

func (o *RouteObfuscator) epoch() int64 {
	return o.now().Unix() / int64(rotation/time.Second)
}

func (o *RouteObfuscator) routes(epoch int64) map[string]string {
	out := make(map[string]string, len(actions))
	for _, action := range actions {
		out[action] = "/" + action
		if o.enabled {
			out[action] = "/" + o.segment(epoch, action)
		}
	}
	return out
}

func (o *RouteObfuscator) segment(epoch int64, action string) string {
	mac := hmac.New(sha256.New, o.key[:])
	mac.Write([]byte{
		byte(epoch >> 56), byte(epoch >> 48), byte(epoch >> 40), byte(epoch >> 32),
		byte(epoch >> 24), byte(epoch >> 16), byte(epoch >> 8), byte(epoch),
	})
	mac.Write([]byte(action))
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

func resolve(path string, routes map[string]string) (action, name string, ok bool) {
	for action, route := range routes {
		if path == route {
			return action, "", true
		}
	}
	name, ok = strings.CutPrefix(path, routes["kubeconfig"]+"/")
	if ok && name != "" {
		return "kubeconfig", name, true
	}
	return "", "", false
}
