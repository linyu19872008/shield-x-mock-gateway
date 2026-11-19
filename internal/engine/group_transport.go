package engine

import "net/http"

// groupTransport injects the optional site-local gateway group header.
// It wraps the existing transport (including fault injection) so behavior is unchanged.
type groupTransport struct {
	base  http.RoundTripper
	group string
}

func withGroupTransport(base http.RoundTripper, group string) http.RoundTripper {
	if group == "" {
		return base
	}
	return &groupTransport{base: base, group: group}
}

func (t *groupTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("X-Shieldx-Gateway-Group", t.group)
	return t.base.RoundTrip(clone)
}
