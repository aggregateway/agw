package agw

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name string
		auth *Authorization
		want string
	}{
		{"none", &Authorization{Type: "none"}, ""},
		{"basic credentials", &Authorization{Type: "basic", Value: "user:pass"}, "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))},
		{"basic encoded", &Authorization{Type: "basic", Value: "abc"}, "Basic abc"},
		{"bearer", &Authorization{Type: "bearer", Value: "secret"}, "Bearer secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authorizationHeader(tt.auth)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestProxyRetriesAndInjectsAuthorization(t *testing.T) {
	var logs bytes.Buffer
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Basic dXNlcjpwYXNz" {
			t.Errorf("first authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":{"message":"Invalid URL (GET /v1/asdfasf)"}}`))
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer backup" {
			t.Errorf("second authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "payload" {
			t.Errorf("body = %q", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("ok"))
	}))
	defer second.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{
			{URL: first.URL, Authorization: &Authorization{Type: "basic", Value: "user:pass"}},
			{URL: second.URL, Authorization: &Authorization{Type: "bearer", Value: "backup"}},
		},
		Client: http.DefaultClient,
		Logger: log.New(&logs, "", 0),
	}
	req := httptest.NewRequest(http.MethodPost, "/test?x=1", strings.NewReader("payload"))
	req.Header.Set("Authorization", "Bearer client-must-not-win")
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated || recorder.Body.String() != "ok" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(logs.String(), "Invalid URL (GET /v1/asdfasf)") {
		t.Fatalf("error response was not logged: %q", logs.String())
	}
}

func TestUpstreamRequestURLPreservesClientPath(t *testing.T) {
	got, err := upstreamRequestURL("https://example.com/v1", "/v1/chat/completions?stream=true")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/v1/chat/completions?stream=true"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNonePreservesClientAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer from-client" {
			t.Errorf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{{URL: server.URL, Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer from-client")
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("response status = %d", recorder.Code)
	}
}

func TestProxyForwardsCustomMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PURGE" {
			t.Errorf("method = %q", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{{URL: server.URL, Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("PURGE", "/resource", strings.NewReader("payload"))
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("response status = %d", recorder.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	proxy := &Proxy{
		Upstreams: []Upstream{{URL: "https://example.com", Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	req.Header.Set("Access-Control-Request-Method", "PURGE")
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("response status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestCORSHeadersAreNotDuplicated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "https://upstream.example")
		w.Header().Add("Access-Control-Allow-Origin", "https://another.example")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{{URL: server.URL, Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
	}
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/resource", nil))

	if got := recorder.Header().Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != "*" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestProxyRequestsUncompressedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Encoding"); got != "identity" {
			t.Errorf("accept encoding = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{{URL: server.URL, Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
	}
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/resource", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("response status = %d", recorder.Code)
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	if err := Run([]string{"-unknown"}); err == nil {
		t.Fatal("Run accepted an unknown flag")
	}
}
