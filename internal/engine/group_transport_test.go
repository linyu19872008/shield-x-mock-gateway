package engine

import (
	"net/http"
	"testing"
)

type captureTransport struct{ header string }

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.header = req.Header.Get("X-Shieldx-Gateway-Group")
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestGroupTransport(t *testing.T) {
	base := &captureTransport{}
	req, _ := http.NewRequest(http.MethodGet, "http://example.test/policies/v1", nil)
	if _, err := withGroupTransport(base, "blue").RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if base.header != "blue" {
		t.Fatalf("group header = %q, want blue", base.header)
	}

	base2 := &captureTransport{}
	if _, err := withGroupTransport(base2, "").RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if base2.header != "" {
		t.Fatalf("empty group should not add header, got %q", base2.header)
	}
}
