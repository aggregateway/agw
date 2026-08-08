package agw

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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

func TestAppSelectorRoutesOnlyCompatibleUpstreams(t *testing.T) {
	selectors := []AppSelector{
		{Name: "codex-luna", Match: AppSelectorMatch{Headers: []HeaderMatch{{Name: "User-Agent", Operator: "contains", Value: "Codex"}}}},
		{Name: "default"},
	}
	upstreams := []Upstream{
		{Name: "d1v-primary", AppSelectors: []string{"codex-luna"}},
		{Name: "deepseek", AppSelectors: []string{"default"}},
		{Name: "d1v-backup", AppSelectors: []string{"codex-luna"}},
	}
	request := http.Header{"User-Agent": []string{"Codex/1.0"}}
	routed, selected, err := routeUpstreams(upstreams, selectors, request, nil)
	if err != nil || selected != "codex-luna" || len(routed) != 2 {
		t.Fatalf("routed upstreams = %#v, selector=%q, error=%v", routed, selected, err)
	}
	if routed[0].Index != 0 || routed[1].Index != 2 {
		t.Fatalf("non-compatible upstream entered retry chain: %#v", routed)
	}

	routed, selected, err = routeUpstreams(upstreams, selectors, http.Header{"User-Agent": []string{"OpenAI/1.0"}}, nil)
	if err != nil || selected != "default" || len(routed) != 1 || routed[0].Upstream.Name != "deepseek" {
		t.Fatalf("default route = %#v, selector=%q, error=%v", routed, selected, err)
	}
}

func TestBodySelectorRoutesByModelField(t *testing.T) {
	selectors := []AppSelector{
		{Name: "deepseek-model", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "model", Operator: "exact", Value: "deepseek"}}}},
		{Name: "default"},
	}
	upstreams := []Upstream{
		{Name: "deepseek", AppSelectors: []string{"deepseek-model"}},
		{Name: "luna", AppSelectors: []string{"default"}},
	}
	routed, selected, err := routeUpstreams(upstreams, selectors, http.Header{}, []byte(`{"model":"deepseek","messages":[]}`))
	if err != nil || selected != "deepseek-model" || len(routed) != 1 || routed[0].Upstream.Name != "deepseek" {
		t.Fatalf("routed upstreams = %#v, selector=%q, error=%v", routed, selected, err)
	}

	routed, selected, err = routeUpstreams(upstreams, selectors, http.Header{}, []byte(`{"model":"gpt-5.6-luna","messages":[]}`))
	if err != nil || selected != "default" || len(routed) != 1 || routed[0].Upstream.Name != "luna" {
		t.Fatalf("default route = %#v, selector=%q, error=%v", routed, selected, err)
	}
}

func TestBodySelectorNestedFieldPrefixAndCase(t *testing.T) {
	selector := AppSelector{Name: "ds", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "metadata.provider", Operator: "prefix", Value: "Deep"}}}}
	if !appSelectorMatches(selector, http.Header{}, []byte(`{"model":"x","metadata":{"provider":"deepseek"}}`)) {
		t.Fatal("nested case-insensitive prefix should match")
	}
	if appSelectorMatches(selector, http.Header{}, []byte(`{"model":"x","metadata":{"provider":"openai"}}`)) {
		t.Fatal("prefix rule matched an unrelated value")
	}
	if appSelectorMatches(selector, http.Header{}, []byte(`not json`)) {
		t.Fatal("non-JSON body must not match")
	}
	if appSelectorMatches(selector, http.Header{}, []byte(`{"metadata":42}`)) {
		t.Fatal("missing nested field must not match")
	}
	if appSelectorMatches(selector, http.Header{}, []byte(`{"metadata":{"provider":null}}`)) {
		t.Fatal("null value must not match")
	}
}

func TestBodySelectorPresentAndValidation(t *testing.T) {
	selector := AppSelector{Name: "stream", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "stream", Operator: "present"}}}}
	if !appSelectorMatches(selector, http.Header{}, []byte(`{"stream":true}`)) {
		t.Fatal("present rule should match when field exists")
	}
	if appSelectorMatches(selector, http.Header{}, []byte(`{"model":"x"}`)) {
		t.Fatal("present rule matched a missing field")
	}
	_, err := parseSettings([]byte(`appSelectors:
  - name: invalid
    match:
      body:
        - field: model
          operator: regex
          value: "["
upstreams:
  - url: https://example.com
    appSelectors: [invalid]
`))
	if err == nil || !strings.Contains(err.Error(), "invalid regex") {
		t.Fatalf("invalid body regex error = %v", err)
	}
}

func TestProxyRoutesByJSONBodyAndForwardsBody(t *testing.T) {
	deepseek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}` {
			t.Errorf("body not forwarded intact: %q", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "deepseek response")
	}))
	defer deepseek.Close()
	luna := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("luna upstream received a deepseek-routed request")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer luna.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{
			{Name: "ds", URL: deepseek.URL, AppSelectors: []string{"ds-model"}, Authorization: &Authorization{Type: "none"}},
			{Name: "luna", URL: luna.URL, AppSelectors: []string{"luna-model"}, Authorization: &Authorization{Type: "none"}},
		},
		AppSelectors: []AppSelector{
			{Name: "ds-model", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "model", Operator: "prefix", Value: "deepseek"}}}},
			{Name: "luna-model", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "model", Operator: "prefix", Value: "gpt"}}}},
		},
		Client: http.DefaultClient,
		Logger: log.New(io.Discard, "", 0),
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "deepseek response" {
		t.Fatalf("routed response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestApplyRewritesSetsAndCreatesFields(t *testing.T) {
	got := applyRewrites([]byte(`{"model":"deepseek","messages":[]}`), []FieldRewrite{
		{Field: "model", Value: "gpt-5.6-luna"},
		{Field: "stream", Value: "true"},
		{Field: "temperature", Value: "0.5"},
		{Field: "metadata.provider", Value: "openai"},
	})
	var doc map[string]any
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatalf("rewritten body is not valid JSON: %v\n%s", err, got)
	}
	if doc["model"] != "gpt-5.6-luna" {
		t.Fatalf("model = %#v", doc["model"])
	}
	if doc["stream"] != true {
		t.Fatalf("stream = %#v", doc["stream"])
	}
	if doc["temperature"] != 0.5 {
		t.Fatalf("temperature = %#v", doc["temperature"])
	}
	metadata, ok := doc["metadata"].(map[string]any)
	if !ok || metadata["provider"] != "openai" {
		t.Fatalf("nested metadata = %#v", doc["metadata"])
	}
}

func TestApplyRewritesLeavesNonObjectBodiesAlone(t *testing.T) {
	rewrites := []FieldRewrite{{Field: "model", Value: "x"}}
	if got := string(applyRewrites([]byte(`[{"model":"a"}]`), rewrites)); got != `[{"model":"a"}]` {
		t.Fatalf("array root was rewritten: %s", got)
	}
	if got := string(applyRewrites([]byte(`not json`), rewrites)); got != `not json` {
		t.Fatalf("non-JSON body was rewritten: %s", got)
	}
	if got := string(applyRewrites([]byte(`{"model":`), rewrites)); got != `{"model":` {
		t.Fatalf("malformed JSON body was rewritten: %s", got)
	}
	if got := string(applyRewrites([]byte(`{"model":"a"}`), nil)); got != `{"model":"a"}` {
		t.Fatalf("empty rewrites changed the body: %s", got)
	}
}

func TestProxyRewritesBodyBeforeForwarding(t *testing.T) {
	var rewrittenBody []byte
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rewrittenBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			t.Errorf("rewritten body is not valid JSON: %v", err)
		}
		if doc["model"] != "gpt-5.6-luna" || doc["stream"] != true {
			t.Errorf("upstream received unrewritten body: %s", body)
		}
		if _, ok := doc["messages"]; !ok {
			t.Errorf("original fields missing after rewrite: %s", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "rewritten ok")
	}))
	defer backup.Close()

	proxy := &Proxy{
		Upstreams: []Upstream{
			{Name: "ds-primary", URL: primary.URL, AppSelectors: []string{"ds"}, Authorization: &Authorization{Type: "none"}},
			{Name: "ds-backup", URL: backup.URL, AppSelectors: []string{"ds"}, Authorization: &Authorization{Type: "none"}},
		},
		AppSelectors: []AppSelector{
			{Name: "ds", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "model", Operator: "prefix", Value: "deepseek"}}}, Rewrite: []FieldRewrite{{Field: "model", Value: "gpt-5.6-luna"}, {Field: "stream", Value: "true"}}},
		},
		Client: http.DefaultClient,
		Logger: log.New(io.Discard, "", 0),
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "rewritten ok" {
		t.Fatalf("routed response = %d %q", recorder.Code, recorder.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(rewrittenBody, &doc); err != nil {
		t.Fatalf("retried body is not valid JSON: %v", err)
	}
	if doc["model"] != "gpt-5.6-luna" {
		t.Fatalf("retry attempt received unrewritten body: %s", rewrittenBody)
	}
}

func TestRewriteValidationRejectsEmptyField(t *testing.T) {
	_, err := parseSettings([]byte(`appSelectors:
  - name: invalid
    rewrite:
      - field: ""
        value: gpt-5.6-luna
upstreams:
  - url: https://example.com
    appSelectors: [invalid]
`))
	if err == nil || !strings.Contains(err.Error(), "rewrite rule") {
		t.Fatalf("empty rewrite field error = %v", err)
	}
}

func TestHeaderMatchCaseSensitivityAndRegex(t *testing.T) {
	headers := http.Header{"User-Agent": []string{"Codex/1.0"}}
	if !headerMatchMatches(HeaderMatch{Name: "User-Agent", Operator: "exact", Value: "codex/1.0"}, headers) {
		t.Fatal("exact match should be case-insensitive by default")
	}
	if headerMatchMatches(HeaderMatch{Name: "User-Agent", Operator: "exact", Value: "codex/1.0", CaseSensitive: true}, headers) {
		t.Fatal("case-sensitive exact match accepted a different case")
	}
	if !headerMatchMatches(HeaderMatch{Name: "User-Agent", Operator: "regex", Value: `^codex/[0-9]+\.[0-9]+$`}, headers) {
		t.Fatal("regex match should be case-insensitive by default")
	}
	if headerMatchMatches(HeaderMatch{Name: "User-Agent", Operator: "regex", Value: `^codex/[0-9]+\.[0-9]+$`, CaseSensitive: true}, headers) {
		t.Fatal("case-sensitive regex accepted a different case")
	}
	_, err := parseSettings([]byte(`appSelectors:
  - name: invalid
    match:
      headers:
        - name: User-Agent
          operator: regex
          value: "["
upstreams:
  - url: https://example.com
    appSelectors: [invalid]
`))
	if err == nil || !strings.Contains(err.Error(), "invalid regex") {
		t.Fatalf("invalid regex error = %v", err)
	}
}

func TestAppSelectorWithoutRulesMatchesAllRequests(t *testing.T) {
	selector := AppSelector{Name: "catch-all"}
	if !appSelectorMatches(selector, http.Header{"User-Agent": []string{"Codex/1.0"}}, nil) {
		t.Fatal("AppSelector without header rules should match every request")
	}
}

func TestAppSelectorValidationRejectsUnknownUpstreamReference(t *testing.T) {
	_, err := parseSettings([]byte(`appSelectors:
  - name: codex
    match:
      headers:
        - name: User-Agent
          operator: contains
          value: Codex
upstreams:
  - name: primary
    url: https://example.com
    appSelectors: [missing]
`))
	if err == nil || !strings.Contains(err.Error(), "unknown app selector") {
		t.Fatalf("unknown selector reference error = %v", err)
	}
}

func TestProxyRetriesWithinMatchedAppSelectorOnly(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	deepseek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("incompatible upstream received routed request")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer deepseek.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "luna backup")
	}))
	defer backup.Close()

	var logs bytes.Buffer
	proxy := &Proxy{
		Upstreams: []Upstream{
			{Name: "luna-primary", URL: primary.URL, AppSelectors: []string{"codex-luna"}, Authorization: &Authorization{Type: "none"}},
			{Name: "ds-video", URL: deepseek.URL, AppSelectors: []string{"deepseek"}, Authorization: &Authorization{Type: "none"}},
			{Name: "luna-backup", URL: backup.URL, AppSelectors: []string{"codex-luna"}, Authorization: &Authorization{Type: "none"}},
		},
		AppSelectors: []AppSelector{
			{Name: "codex-luna", Match: AppSelectorMatch{Headers: []HeaderMatch{{Name: "User-Agent", Operator: "contains", Value: "Codex"}}}},
			{Name: "deepseek", Match: AppSelectorMatch{Headers: []HeaderMatch{{Name: "User-Agent", Operator: "contains", Value: "DeepSeek"}}}},
		},
		Client: http.DefaultClient,
		Logger: log.New(&logs, "", 0),
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("payload"))
	request.Header.Set("User-Agent", "Codex/1.0")
	proxy.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "luna backup" {
		t.Fatalf("routed response = %d %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(logs.String(), "UPSTREAM[2:luna-backup] | ATTEMPT") || strings.Contains(logs.String(), "UPSTREAM[1:ds-video] | ATTEMPT") {
		t.Fatalf("retry chain crossed AppSelector boundary: %q", logs.String())
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
	req := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(`{"debug":true,"appSelectors":[{"name":"codex","match":{"headers":[{"name":"User-Agent","operator":"regex","value":"^Codex","caseSensitive":true}]}}],"upstreams":[{"url":"https://new.example","appSelectors":["codex"],"authorization":{"type":"none"}}]}`))
	recorder := httptest.NewRecorder()

	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("response status = %d", recorder.Code)
	}
	if !proxy.Debug || len(proxy.Upstreams) != 1 || proxy.Upstreams[0].URL != "https://new.example" || len(proxy.AppSelectors) != 1 || !proxy.AppSelectors[0].Match.Headers[0].CaseSensitive {
		t.Fatalf("runtime settings were not updated: debug=%t upstreams=%#v selectors=%#v", proxy.Debug, proxy.Upstreams, proxy.AppSelectors)
	}
	settings, err := loadSettings(configPath)
	if err != nil || !settings.Debug || settings.Upstreams[0].URL != "https://new.example" || !settings.AppSelectors[0].Match.Headers[0].CaseSensitive {
		t.Fatalf("saved settings = %#v, error = %v", settings, err)
	}
}

func TestSessionHubTracksLifecycleAndRedactsHeaders(t *testing.T) {
	hub := newSessionHub()
	defer hub.close()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("payload"))
	req.Header.Set("Session-Id", "session-primary")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Client-Request-Id", "request-1")

	tracked := hub.start(req)
	cards := hub.cards()
	if len(cards) != 1 || cards[0].State != "connecting" || len(cards[0].ID) != 36 || cards[0].ID[14] != '7' {
		t.Fatalf("initial cards = %#v", cards)
	}
	tracked.connected(http.StatusOK)
	requestBody := []byte(strings.Repeat("request-body-", 2048))
	tracked.setRequestBody("application/json", requestBody)
	tracked.setContentType("text/event-stream")
	tracked.captureResponse([]byte("data: hello\\n\\n"))
	tracked.complete(http.StatusOK, 2048, nil)

	cards = hub.cards()
	if cards[0].State != "completed" || cards[0].Status != "200" || cards[0].Latest.Bytes != "2.0 KB" {
		t.Fatalf("completed card = %#v", cards[0])
	}
	content, err := hub.renderCards()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "secret-token") || !strings.Contains(content, "[redacted]") {
		t.Fatalf("session card exposes a sensitive header: %s", content)
	}
	if strings.Contains(content, "data: hello") || strings.Contains(content, "request-body-") {
		t.Fatalf("session card embeds payload data: %s", content)
	}
	if !strings.Contains(content, `data-session-payload="request"`) || !strings.Contains(content, `data-session-payload="response"`) {
		t.Fatalf("session card does not include payload loaders: %s", content)
	}
	if !strings.Contains(content, `<span class="session-metric session-transfer"><small>latest transfer</small><strong>2.0 KB</strong></span>`) {
		t.Fatalf("session card does not include the latest transfer summary: %s", content)
	}
	capturedRequestBody, found, err := hub.readPayload(cards[0].ID, "request", 0)
	if err != nil || !found || !bytes.Equal(capturedRequestBody, requestBody) {
		t.Fatalf("request payload length = %d, found=%t, err=%v", len(capturedRequestBody), found, err)
	}
	responseBody, found, err := hub.readPayload(cards[0].ID, "response", 0)
	if err != nil || !found || string(responseBody) != "data: hello\\n\\n" {
		t.Fatalf("response payload = %q, found=%t, err=%v", responseBody, found, err)
	}
}

func TestSessionHubUsesSeparateServerUUIDv7s(t *testing.T) {
	hub := newSessionHub()
	defer hub.close()
	first := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	second := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	first.Header.Set("Session-Id", "client-reused-session")
	second.Header.Set("Session-Id", "client-reused-session")
	hub.start(first)
	hub.start(second)

	cards := hub.cards()
	if len(cards) != 2 || cards[0].ID == cards[1].ID {
		t.Fatalf("server sessions were grouped: %#v", cards)
	}
	for _, card := range cards {
		if len(card.ID) != 36 || card.ID[14] != '7' || (card.ID[19] != '8' && card.ID[19] != '9' && card.ID[19] != 'a' && card.ID[19] != 'b') {
			t.Fatalf("session ID is not UUIDv7: %q", card.ID)
		}
	}
}

func TestProxyInterceptionPreservesSSEStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for _, chunk := range []string{"data: first\\n\\n", "data: second\\n\\n"} {
			_, _ = io.WriteString(w, chunk)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	hub := newSessionHub()
	defer hub.close()
	proxy := &Proxy{Upstreams: []Upstream{{URL: upstream.URL, Authorization: &Authorization{Type: "none"}}}, Client: http.DefaultClient, Logger: log.New(io.Discard, "", 0), Sessions: hub}
	request := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	request.Header.Set("Session-Id", "stream-session")
	recorder := httptest.NewRecorder()
	requestLogger(proxy.Logger, proxy).ServeHTTP(recorder, request)

	want := "data: first\\n\\ndata: second\\n\\n"
	if got := recorder.Body.String(); got != want {
		t.Fatalf("client stream = %q, want %q", got, want)
	}
	cards := hub.cards()
	responseBody, found, err := hub.readPayload(cards[0].ID, "response", 0)
	if err != nil || !found || string(responseBody) != want {
		t.Fatalf("intercepted stream = %q, found=%t, err=%v", responseBody, found, err)
	}
}

func TestProxySessionCardShowsAppSelectorAndUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	hub := newSessionHub()
	defer hub.close()
	proxy := &Proxy{
		Upstreams:    []Upstream{{Name: "d1v.ai", URL: upstream.URL, AppSelectors: []string{"codex-luna"}, Authorization: &Authorization{Type: "none"}}},
		AppSelectors: []AppSelector{{Name: "codex-luna", Match: AppSelectorMatch{Headers: []HeaderMatch{{Name: "User-Agent", Operator: "contains", Value: "Codex"}}}}},
		Client:       http.DefaultClient,
		Logger:       log.New(io.Discard, "", 0),
		Sessions:     hub,
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.6-luna","messages":[]}`))
	request.Header.Set("User-Agent", "Codex/1.0")
	requestLogger(proxy.Logger, proxy).ServeHTTP(httptest.NewRecorder(), request)

	content, err := hub.renderCards()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "codex-luna") || !strings.Contains(content, "UPSTREAM[0:d1v.ai]") {
		t.Fatalf("session card route details missing: %s", content)
	}
	if !strings.Contains(content, `class="session-model">gpt-5.6-luna<`) {
		t.Fatalf("session card does not show the request model: %s", content)
	}
}

func TestSessionCardShowsRewrittenModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	hub := newSessionHub()
	defer hub.close()
	proxy := &Proxy{
		Upstreams:    []Upstream{{URL: upstream.URL, AppSelectors: []string{"ds"}, Authorization: &Authorization{Type: "none"}}},
		AppSelectors: []AppSelector{{Name: "ds", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "model", Operator: "prefix", Value: "deepseek"}}}, Rewrite: []FieldRewrite{{Field: "model", Value: "gpt-5.6-luna"}}}},
		Client:       http.DefaultClient,
		Logger:       log.New(io.Discard, "", 0),
		Sessions:     hub,
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-v4-flash","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	requestLogger(proxy.Logger, proxy).ServeHTTP(httptest.NewRecorder(), request)

	content, err := hub.renderCards()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, `class="session-model">deepseek-v4-flash =&gt; gpt-5.6-luna<`) {
		t.Fatalf("session card does not show the original => rewritten model: %s", content)
	}
	if strings.Contains(content, `class="session-model">deepseek-v4-flash<`) {
		t.Fatalf("session card hides the rewrite: %s", content)
	}
}

func TestSessionResponsePayloadReadsLatestDiskBytes(t *testing.T) {
	hub := newSessionHub()
	defer hub.close()
	request := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	request.Header.Set("Session-Id", "preview-tail")
	tracked := hub.start(request)
	tracked.setContentType("text/event-stream")
	tracked.captureResponse([]byte(strings.Repeat("a", 128<<10) + "tail"))

	cards := hub.cards()
	payload, found, err := hub.readPayload(cards[0].ID, "response", 64<<10)
	if err != nil || !found {
		t.Fatalf("response payload found=%t, err=%v", found, err)
	}
	if got := len(payload); got != 64<<10 {
		t.Fatalf("tail length = %d, want %d", got, 64<<10)
	}
	if !strings.HasSuffix(string(payload), "tail") {
		t.Fatalf("tail does not retain latest response data")
	}
}

func TestConfigPageDefaultsToDarkSessionJournal(t *testing.T) {
	recorder := httptest.NewRecorder()
	serveConfigPage(recorder, httptest.NewRequest(http.MethodGet, "/", nil), nil, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("config page status = %d", recorder.Code)
	}
	content := recorder.Body.String()
	for _, expected := range []string{"agw-theme", "'dark'", "theme-toggle", "telemetry-tabbar", "SSE connected", "sessions-panel", "logs-panel", "aria-selected=\"true\"", "AppSelector registry", "Compatible AppSelectors", "selector-workspace", "selector-table-head", "Header match<br>rules", "selector-count", "updateSelectorSummary", "match-value-field", "match-value-actions", "selector-no-rules", "No rules - matches all requests", ">Actions<", "data-selector", "data-drop-zone", "drop-indicator", "松手后放到这里", "data-duplicate-row", "data-duplicate-selector"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config page missing %q", expected)
		}
	}
	for _, expected := range []string{"Body match<br>rules", "data-selector-body-matches", "data-body-field", "No body rules", "data-add-body-match", "data-delete-body-match"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config page missing body match UI %q", expected)
		}
	}
	for _, expected := range []string{"Request rewrite<br>rules", "data-selector-rewrites", "data-rewrite-field", "data-rewrite-value", "No rewrites", "data-add-rewrite", "data-delete-rewrite"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config page missing rewrite UI %q", expected)
		}
	}
	for _, expected := range []string{"reconcileSessionCards", "updateSessionCard", "refreshResponsePreviews", "EventSource('/sessions/stream')"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config page missing session journal JS %q", expected)
		}
	}
	workspace := strings.Index(content, `aria-labelledby="routing-title"`)
	addUpstream := strings.Index(content, `id="add-upstream"`)
	if workspace < 0 || addUpstream < workspace {
		t.Fatalf("upstream add button is outside its routing container")
	}
	routingEnd := strings.Index(content[workspace:], "</section>")
	selectorWorkspace := strings.Index(content, `class="workspace selector-workspace"`)
	if routingEnd < 0 || selectorWorkspace < 0 || selectorWorkspace <= workspace+routingEnd {
		t.Fatalf("AppSelector registry is still nested in the upstream routing container")
	}
}

func TestConfigPageRendersBodyMatchRules(t *testing.T) {
	recorder := httptest.NewRecorder()
	serveConfigPage(recorder, httptest.NewRequest(http.MethodGet, "/", nil), []AppSelector{
		{Name: "deepseek", Match: AppSelectorMatch{Body: []BodyMatch{{Field: "model", Operator: "prefix", Value: "deepseek", CaseSensitive: true}}}, Rewrite: []FieldRewrite{{Field: "model", Value: "gpt-5.6-luna"}}},
	}, false)
	content := recorder.Body.String()
	for _, expected := range []string{`value="deepseek"`, `value="model"`, `data-selector-body-match`, `data-case-sensitive="true"`} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config page body rules missing %q", expected)
		}
	}
	for _, expected := range []string{`value="gpt-5.6-luna"`, `data-selector-rewrite`} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config page rewrite rules missing %q", expected)
		}
	}
}

func TestConfigFragmentRendersMultiSelectForAppSelectors(t *testing.T) {
	recorder := httptest.NewRecorder()
	serveConfigFragment(recorder, []Upstream{
		{Name: "primary", URL: "https://example.com/v1", AppSelectors: []string{"codex", "fallback"}},
	})
	content := recorder.Body.String()
	for _, expected := range []string{"data-multi-select", "data-ms-trigger", "data-ms-menu", `value="codex, fallback"`} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config fragment missing %q", expected)
		}
	}
	if strings.Contains(content, `placeholder="codex, default"`) {
		t.Fatalf("config fragment still uses a free-text app selector input")
	}
}

func TestWriteSessionSSERemovesCarriageReturns(t *testing.T) {
	var output bytes.Buffer
	if err := writeSessionSSE(&output, "first\r\nsecond"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\r") || !strings.Contains(output.String(), "event: sessions\n") {
		t.Fatalf("invalid session SSE frame: %q", output.String())
	}
}

func TestSessionRouteShowsTrackedProxyRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	defer upstream.Close()

	hub := newSessionHub()
	defer hub.close()
	proxy := &Proxy{
		Upstreams: []Upstream{{URL: upstream.URL, Authorization: &Authorization{Type: "none"}}},
		Client:    http.DefaultClient,
		Logger:    log.New(io.Discard, "", 0),
		Sessions:  hub,
	}
	handler := requestLogger(proxy.Logger, proxy)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("payload"))
	request.Header.Set("Session-Id", "session-route")
	request.Header.Set("Authorization", "Bearer client-secret")
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sessions", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Intercepted request") {
		t.Fatalf("session route response = %d %q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "client-secret") || !strings.Contains(recorder.Body.String(), "[redacted]") {
		t.Fatalf("session route exposes authorization: %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), ">payload<") {
		t.Fatalf("session route embeds request body: %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Gateway events") || !strings.Contains(recorder.Body.String(), "attempt") || !strings.Contains(recorder.Body.String(), "response") {
		t.Fatalf("session route does not show gateway events: %q", recorder.Body.String())
	}
	cards := hub.cards()
	requestPayload := httptest.NewRecorder()
	proxy.ServeHTTP(requestPayload, httptest.NewRequest(http.MethodGet, "/sessions/"+cards[0].ID+"/request", nil))
	if requestPayload.Code != http.StatusOK || requestPayload.Body.String() != "payload" {
		t.Fatalf("request payload response = %d %q", requestPayload.Code, requestPayload.Body.String())
	}
	responsePayload := httptest.NewRecorder()
	proxy.ServeHTTP(responsePayload, httptest.NewRequest(http.MethodGet, "/sessions/"+cards[0].ID+"/response", nil))
	if responsePayload.Code != http.StatusOK || responsePayload.Body.String() != "accepted" {
		t.Fatalf("response payload response = %d %q", responsePayload.Code, responsePayload.Body.String())
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
