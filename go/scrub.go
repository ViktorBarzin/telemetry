package telemetry

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Redacted replaces a secret in a log line.
const Redacted = "[REDACTED]"

// minSecretLength keeps short values such as "dev" from being masked.
const minSecretLength = 8

var (
	bearerRE = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=\-]+`)
	basicRE  = regexp.MustCompile(`(?i)\bbasic\s+[A-Za-z0-9+/]{8,}={0,2}`)
	jwtRE    = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{2,}`)
	pemRE    = regexp.MustCompile(`(?s)-----BEGIN[^-]+-----.*?-----END[^-]+-----`)

	registryMu sync.RWMutex
	registry   []string
)

// RegisterSecret masks value in every Handler's output from now on. New
// registers the write key.
func RegisterSecret(value string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = longestFirst(append(registry, value))
}

func registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry
}

func longestFirst(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if len(v) >= minSecretLength && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	// Longest first, so a secret containing another is masked whole.
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func scrub(text string, own []string) string {
	for _, list := range [][]string{own, registered()} {
		for _, secret := range list {
			text = strings.ReplaceAll(text, secret, Redacted)
		}
	}
	text = bearerRE.ReplaceAllString(text, "Bearer "+Redacted)
	text = basicRE.ReplaceAllString(text, "Basic "+Redacted)
	text = jwtRE.ReplaceAllString(text, Redacted)
	return pemRE.ReplaceAllString(text, Redacted)
}
