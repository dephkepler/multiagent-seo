package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestDisallowedIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"169.254.169.254", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, c := range cases {
		if got := DisallowedIP(netip.MustParseAddr(c.ip)); got != c.want {
			t.Errorf("DisallowedIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestBlockPrivateIPsBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := New(WithMaxRedirects(5), BlockPrivateIPs())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if _, err := client.Do(req); err == nil {
		t.Fatal("expected guard to block loopback dial, got nil error")
	}
}

func TestAllowLoopbackPermitsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := New(BlockPrivateIPs(), AllowLoopback())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("loopback should be allowed: %v", err)
	}
	resp.Body.Close()
}

func TestUserAgentInjectedAndOverridable(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := New(BlockPrivateIPs(), AllowLoopback())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if ua := <-got; ua != DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", ua, DefaultUserAgent)
	}

	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	req2.Header.Set("User-Agent", "custom-ua")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp2.Body.Close()
	if ua := <-got; ua != "custom-ua" {
		t.Errorf("override User-Agent = %q, want custom-ua", ua)
	}
}
