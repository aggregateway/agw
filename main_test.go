package agw

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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
	if !strings.Contains(logs.String(), "| UPSTREAM[0] | RESPONSE | 502 Bad Gateway") || !strings.Contains(logs.String(), "Invalid URL (GET /v1/asdfasf)") {
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

func TestDefaultHTTPClientHasNoOverallTimeout(t *testing.T) {
	if got := newHTTPClient(0).Timeout; got != 0 {
		t.Fatalf("default timeout = %s, want 0", got)
	}
	if got := newHTTPClient(2 * time.Minute).Timeout; got != 2*time.Minute {
		t.Fatalf("configured timeout = %s", got)
	}
}

func TestParseSettingsSupportsLegacyAndObjectFormats(t *testing.T) {
	legacy, err := parseSettings([]byte("- url: https://example.com/v1\n  authorization:\n    type: none\n"))
	if err != nil || len(legacy.Upstreams) != 1 || legacy.Debug {
		t.Fatalf("legacy settings = %#v, error = %v", legacy, err)
	}

	modern, err := parseSettings([]byte("debug: true\nupstreams:\n- url: https://example.com/v1\n  authorization:\n    type: bearer\n    value: token\n"))
	if err != nil || !modern.Debug || len(modern.Upstreams) != 1 {
		t.Fatalf("modern settings = %#v, error = %v", modern, err)
	}
}

func TestUpdateConfigChangesRuntimeSettings(t *testing.T) {
	configPath := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configPath, []byte("debug: false\nupstreams:\n- url: https://old.example\n  authorization:\n    type: none\n"), 0600); err != nil {
		t.Fatal(err)
	}
	proxy := &Proxy{
		Upstreams: []Upstream{{URL: "https://old.example", Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
		Config:    configPath,
	}
	req := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(`{"debug":true,"upstreams":[{"url":"https://new.example","authorization":{"type":"none"}}]}`))
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("response status = %d", recorder.Code)
	}
	if !proxy.Debug || len(proxy.Upstreams) != 1 || proxy.Upstreams[0].URL != "https://new.example" {
		t.Fatalf("runtime settings were not updated: debug=%t upstreams=%#v", proxy.Debug, proxy.Upstreams)
	}
	settings, err := loadSettings(configPath)
	if err != nil || !settings.Debug || settings.Upstreams[0].URL != "https://new.example" {
		t.Fatalf("saved settings = %#v, error = %v", settings, err)
	}
}

func TestRequestLoggerUsesGinLikeAccessFormat(t *testing.T) {
	var logs bytes.Buffer
	logger := log.New(&logs, "[AGW] ", log.LstdFlags)
	handler := requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses?stream=true", strings.NewReader("payload"))
	req.RemoteAddr = "192.0.2.10:1234"
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)
	line := logs.String()
	for _, expected := range []string{"| 201 |", "192.0.2.10", "POST    /v1/responses?stream=true", "| 2B"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("access log %q does not contain %q", line, expected)
		}
	}
}
