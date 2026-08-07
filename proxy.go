package agw

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type sessionContextKey struct{}

type Authorization struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

type Upstream struct {
	Name          string         `yaml:"name" json:"name"`
	URL           string         `yaml:"url"`
	Authorization *Authorization `yaml:"authorization"`
	AppSelectors  []string       `yaml:"appSelectors,omitempty" json:"appSelectors,omitempty"`
}

type HeaderMatch struct {
	Name          string `yaml:"name" json:"name"`
	Operator      string `yaml:"operator" json:"operator"`
	Value         string `yaml:"value" json:"value"`
	CaseSensitive bool   `yaml:"caseSensitive,omitempty" json:"caseSensitive,omitempty"`
	regex         *regexp.Regexp
}

type AppSelector struct {
	Name  string           `yaml:"name" json:"name"`
	Match AppSelectorMatch `yaml:"match,omitempty" json:"match,omitempty"`
}

type AppSelectorMatch struct {
	Headers []HeaderMatch `yaml:"headers,omitempty" json:"headers,omitempty"`
}

type Settings struct {
	Debug        bool          `yaml:"debug" json:"debug"`
	AppSelectors []AppSelector `yaml:"appSelectors,omitempty" json:"appSelectors,omitempty"`
	Upstreams    []Upstream    `yaml:"upstreams" json:"upstreams"`
}

type Proxy struct {
	Upstreams    []Upstream
	Client       *http.Client
	Logger       *log.Logger
	Config       string
	LogHub       *logHub
	Sessions     *sessionHub
	Debug        bool
	AppSelectors []AppSelector
	Mu           sync.RWMutex
}

func loadConfig(path string) ([]Upstream, error) {
	settings, err := loadSettings(path)
	if err != nil {
		return nil, err
	}
	return settings.Upstreams, nil
}

func loadSettings(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}
	var legacy []Upstream
	if err := yaml.Unmarshal(data, &legacy); err == nil {
		settings := Settings{Upstreams: legacy}
		if err := validateSettings(&settings); err != nil {
			return Settings{}, err
		}
		return settings, nil
	}
	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("parse config: %w", err)
	}
	if err := validateSettings(&settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func validateSettings(settings *Settings) error {
	selectorNames := make(map[string]struct{}, len(settings.AppSelectors))
	for i := range settings.AppSelectors {
		selector := &settings.AppSelectors[i]
		name := strings.TrimSpace(selector.Name)
		if name == "" || strings.ContainsAny(name, "\r\n") {
			return fmt.Errorf("app selector %d has an invalid name", i+1)
		}
		if _, exists := selectorNames[name]; exists {
			return fmt.Errorf("app selector name %q is duplicated", name)
		}
		selectorNames[name] = struct{}{}
		for j := range selector.Match.Headers {
			matcher := &selector.Match.Headers[j]
			if strings.TrimSpace(matcher.Name) == "" || strings.ContainsAny(matcher.Name, "\r\n") || strings.ContainsAny(matcher.Value, "\r\n") {
				return fmt.Errorf("app selector %q header %d is invalid", name, j+1)
			}
			if !supportedHeaderOperator(matcher.Operator) {
				return fmt.Errorf("app selector %q has unsupported header operator %q", name, matcher.Operator)
			}
			if err := compileHeaderMatcher(matcher); err != nil {
				return fmt.Errorf("app selector %q header %d: %w", name, j+1, err)
			}
		}
	}
	return validateUpstreams(settings.Upstreams, selectorNames)
}

func validateUpstreams(upstreams []Upstream, selectorNames map[string]struct{}) error {
	if len(upstreams) == 0 {
		return errors.New("config must contain at least one upstream")
	}
	for i := range upstreams {
		u, err := url.Parse(upstreams[i].URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("upstream %d has invalid url %q", i+1, upstreams[i].URL)
		}
		if upstreams[i].Authorization != nil && upstreams[i].Authorization.Type == "" {
			return fmt.Errorf("upstream %d authorization type is empty", i+1)
		}
		if upstreams[i].Authorization != nil && !supportedAuthorizationType(upstreams[i].Authorization.Type) {
			return fmt.Errorf("upstream %d has unsupported authorization type %q", i+1, upstreams[i].Authorization.Type)
		}
		for _, selectorName := range upstreams[i].AppSelectors {
			selectorName = strings.TrimSpace(selectorName)
			if selectorName == "" {
				return fmt.Errorf("upstream %d has an empty app selector", i+1)
			}
			if _, exists := selectorNames[selectorName]; !exists {
				return fmt.Errorf("upstream %d references unknown app selector %q", i+1, selectorName)
			}
		}
	}
	return nil
}

func supportedAuthorizationType(authType string) bool {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case "none", "basic", "bearer":
		return true
	default:
		return false
	}
}

func supportedHeaderOperator(operator string) bool {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "exact", "prefix", "contains", "present", "regex":
		return true
	default:
		return false
	}
}

func compileHeaderMatcher(matcher *HeaderMatch) error {
	matcher.regex = nil
	if !strings.EqualFold(matcher.Operator, "regex") {
		return nil
	}
	if matcher.Value == "" {
		return errors.New("regex value is empty")
	}
	pattern := matcher.Value
	if !matcher.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}
	matcher.regex = compiled
	return nil
}

func authorizationHeader(auth *Authorization) (string, error) {
	if auth == nil || strings.EqualFold(auth.Type, "none") {
		return "", nil
	}
	value := strings.TrimSpace(auth.Value)
	if value == "" {
		return "", errors.New("authorization value is empty")
	}
	if strings.Contains(value, " ") && (strings.HasPrefix(strings.ToLower(value), "basic ") || strings.HasPrefix(strings.ToLower(value), "bearer ")) {
		return value, nil
	}
	switch strings.ToLower(auth.Type) {
	case "basic":
		if strings.Contains(value, ":") {
			return "Basic " + base64.StdEncoding.EncodeToString([]byte(value)), nil
		}
		return "Basic " + value, nil
	case "bearer":
		return "Bearer " + value, nil
	default:
		return "", fmt.Errorf("unsupported authorization type %q", auth.Type)
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func upstreamID(index int, upstream Upstream) string {
	if upstream.Name == "" {
		return fmt.Sprintf("UPSTREAM[%d]", index)
	}
	return fmt.Sprintf("UPSTREAM[%d:%s]", index, upstream.Name)
}

type routedUpstream struct {
	Index int
	Upstream
}

func headerMatchMatches(matcher HeaderMatch, headers http.Header) bool {
	values := headers.Values(matcher.Name)
	if strings.EqualFold(matcher.Operator, "present") {
		return len(values) > 0
	}
	needle := matcher.Value
	if !matcher.CaseSensitive {
		needle = strings.ToLower(needle)
	}
	for _, value := range values {
		comparison := value
		if !matcher.CaseSensitive {
			comparison = strings.ToLower(comparison)
		}
		switch strings.ToLower(strings.TrimSpace(matcher.Operator)) {
		case "exact":
			if comparison == needle {
				return true
			}
		case "prefix":
			if strings.HasPrefix(comparison, needle) {
				return true
			}
		case "contains":
			if strings.Contains(comparison, needle) {
				return true
			}
		case "regex":
			compiled := matcher.regex
			if compiled == nil {
				if err := compileHeaderMatcher(&matcher); err != nil {
					continue
				}
				compiled = matcher.regex
			}
			if compiled.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func appSelectorMatches(selector AppSelector, headers http.Header) bool {
	for _, matcher := range selector.Match.Headers {
		if !headerMatchMatches(matcher, headers) {
			return false
		}
	}
	return true
}

func upstreamSupportsSelector(upstream Upstream, selector string) bool {
	for _, name := range upstream.AppSelectors {
		if name == selector {
			return true
		}
	}
	return false
}

func routeUpstreams(upstreams []Upstream, selectors []AppSelector, headers http.Header) ([]routedUpstream, string, error) {
	if len(selectors) == 0 {
		routed := make([]routedUpstream, 0, len(upstreams))
		for index, upstream := range upstreams {
			routed = append(routed, routedUpstream{Index: index, Upstream: upstream})
		}
		return routed, "", nil
	}
	for _, selector := range selectors {
		if !appSelectorMatches(selector, headers) {
			continue
		}
		routed := make([]routedUpstream, 0, len(upstreams))
		for index, upstream := range upstreams {
			if upstreamSupportsSelector(upstream, selector.Name) {
				routed = append(routed, routedUpstream{Index: index, Upstream: upstream})
			}
		}
		if len(routed) == 0 {
			return nil, selector.Name, fmt.Errorf("no upstream is compatible with app selector %q", selector.Name)
		}
		return routed, selector.Name, nil
	}
	return nil, "", errors.New("no app selector matched the request headers")
}

func upstreamRequestURL(upstreamURL, requestURI string) (string, error) {
	base, err := url.Parse(upstreamURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid upstream url %q", upstreamURL)
	}
	// The upstream URL supplies the scheme and host only. Preserve the client's
	// complete path and query exactly as received.
	base.Path = ""
	base.RawPath = ""
	base.RawQuery = ""
	return strings.TrimRight(base.String(), "/") + requestURI, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	p.Mu.RLock()
	upstreams := append([]Upstream(nil), p.Upstreams...)
	appSelectors := append([]AppSelector(nil), p.AppSelectors...)
	debug := p.Debug
	p.Mu.RUnlock()

	if r.URL.Path == "/" && r.Method == http.MethodGet {
		serveConfigPage(w, r, appSelectors, debug)
		return
	}
	if r.URL.Path == "/config" && r.Method == http.MethodGet {
		serveConfigFragment(w, upstreams)
		return
	}
	if r.URL.Path == "/logs" && r.Method == http.MethodGet {
		p.serveLogs(w, r)
		return
	}
	if (r.URL.Path == "/sessions" || r.URL.Path == "/sessions/stream") && r.Method == http.MethodGet {
		p.serveSessions(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/sessions/") && r.Method == http.MethodGet {
		p.serveSessionPayload(w, r)
		return
	}
	if r.URL.Path == "/config" && r.Method == http.MethodPut {
		p.updateConfig(w, r)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()
	session := trackedSessionFromContext(r.Context())
	if session != nil {
		session.setRequestBody(r.Header.Get("Content-Type"), body)
	}
	routedUpstreams, appSelector, routeErr := routeUpstreams(upstreams, appSelectors, r.Header)
	if routeErr != nil {
		p.Logger.Printf("| ROUTER | NO_MATCH | %v", routeErr)
		if session != nil {
			session.setAppSelector(appSelector)
			session.addEvent("route error", routeErr.Error())
		}
		http.Error(w, "no compatible upstream route: "+routeErr.Error(), http.StatusServiceUnavailable)
		return
	}
	if appSelector != "" {
		p.Logger.Printf("| ROUTER | MATCH | appSelector=%s | upstreams=%d", appSelector, len(routedUpstreams))
		if session != nil {
			session.setAppSelector(appSelector)
			session.addEvent("route", "appSelector="+appSelector)
		}
	}

	var lastErr error
	for attempt, candidate := range routedUpstreams {
		upstream := candidate.Upstream
		upstreamLabel := upstreamID(candidate.Index, upstream)
		p.Logger.Printf("| %s | ATTEMPT | %s %s", upstreamLabel, r.Method, r.URL.RequestURI())
		if session != nil {
			session.setUpstream(upstreamLabel)
			session.addEvent("attempt", upstreamLabel)
		}
		header, err := authorizationHeader(upstream.Authorization)
		if err != nil {
			lastErr = err
			p.Logger.Printf("| %s | CONFIG_ERROR | %v", upstreamLabel, err)
			if session != nil {
				session.addEvent("config error", err.Error())
			}
			continue
		}
		target, err := upstreamRequestURL(upstream.URL, r.URL.RequestURI())
		if err != nil {
			lastErr = err
			p.Logger.Printf("| %s | TARGET_ERROR | %v", upstreamLabel, err)
			if session != nil {
				session.addEvent("target error", err.Error())
			}
			continue
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			p.Logger.Printf("| %s | REQUEST_ERROR | %v", upstreamLabel, err)
			if session != nil {
				session.addEvent("request error", err.Error())
			}
			continue
		}
		copyHeaders(req.Header, r.Header)
		// Keep error bodies readable in logs and let the proxy return plain text.
		req.Header.Set("Accept-Encoding", "identity")
		if upstream.Authorization == nil || !strings.EqualFold(upstream.Authorization.Type, "none") {
			req.Header.Del("Authorization")
			req.Header.Set("Authorization", header)
		}

		resp, err := p.Client.Do(req)
		if err != nil {
			lastErr = err
			p.Logger.Printf("| %s | TRANSPORT_ERROR | %v", upstreamLabel, err)
			if session != nil {
				session.addEvent("transport error", err.Error())
			}
			continue
		}
		if resp.StatusCode >= http.StatusBadRequest {
			errorBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			}
			p.Logger.Printf("| %s | RESPONSE | %s | %s %s | %s", upstreamLabel, resp.Status, r.Method, target, strings.TrimSpace(string(errorBody)))
			if session != nil {
				session.addEvent("response", upstreamLabel+" · "+resp.Status)
			}
			if retryableStatus(resp.StatusCode) && attempt < len(routedUpstreams)-1 {
				next := routedUpstreams[attempt+1]
				p.Logger.Printf("| %s | RETRY | next=%s", upstreamLabel, upstreamID(next.Index, next.Upstream))
				if session != nil {
					session.addEvent("retry", "next="+upstreamID(next.Index, next.Upstream))
				}
				continue
			}
			if session != nil {
				session.setContentType(resp.Header.Get("Content-Type"))
				session.captureResponse(errorBody)
			}
			copyResponseHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(errorBody)
			return
		}
		p.Logger.Printf("| %s | RESPONSE | %s | using response", upstreamLabel, resp.Status)
		if session != nil {
			session.setContentType(resp.Header.Get("Content-Type"))
			session.addEvent("response", upstreamLabel+" · "+resp.Status)
		}
		copyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		responseWriter := io.Writer(streamResponseWriter{ResponseWriter: w})
		if session != nil {
			responseWriter = io.MultiWriter(responseWriter, sessionResponseWriter{session: session})
		}
		if _, err := io.Copy(responseWriter, resp.Body); err != nil {
			p.Logger.Printf("| %s | STREAM_ERROR | %v", upstreamLabel, err)
			if session != nil {
				session.addEvent("stream error", err.Error())
			}
		}
		resp.Body.Close()
		return
	}

	if r.Context().Err() != nil {
		return
	}
	p.Logger.Printf("| UPSTREAM | EXHAUSTED | last_error=%v", lastErr)
	if session != nil {
		session.addEvent("exhausted", fmt.Sprint(lastErr))
	}
	http.Error(w, "all upstreams failed", http.StatusBadGateway)
}

type sessionResponseWriter struct {
	session *trackedSession
}

func (w sessionResponseWriter) Write(data []byte) (int, error) {
	w.session.captureResponse(data)
	return len(data), nil
}

type streamResponseWriter struct {
	http.ResponseWriter
}

func (w streamResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err == nil {
		if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	return n, err
}

func trackedSessionFromContext(ctx context.Context) *trackedSession {
	session, _ := ctx.Value(sessionContextKey{}).(*trackedSession)
	return session
}

func isManagementRequest(r *http.Request) bool {
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		return true
	}
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		return true
	}
	if r.URL.Path == "/config" && (r.Method == http.MethodGet || r.Method == http.MethodPut) {
		return true
	}
	return (r.URL.Path == "/logs" || r.URL.Path == "/sessions" || strings.HasPrefix(r.URL.Path, "/sessions/")) && r.Method == http.MethodGet
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (p *Proxy) serveLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	client, history := p.LogHub.subscribe()
	defer p.LogHub.unsubscribe(client)
	for _, message := range history {
		if err := writeSSE(w, message); err != nil {
			return
		}
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case message := <-client:
			if err := writeSSE(w, message); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (p *Proxy) updateConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read config", http.StatusBadRequest)
		return
	}
	settings, err := parseSettings(data)
	if err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusBadRequest)
		return
	}
	p.Mu.RLock()
	if settings.Upstreams == nil {
		settings.Upstreams = append([]Upstream(nil), p.Upstreams...)
	}
	if settings.AppSelectors == nil {
		settings.AppSelectors = append([]AppSelector(nil), p.AppSelectors...)
	}
	p.Mu.RUnlock()
	encoded, err := yaml.Marshal(settings)
	if err != nil {
		http.Error(w, "failed to encode config", http.StatusInternalServerError)
		return
	}
	p.Mu.Lock()
	err = os.WriteFile(p.Config, encoded, 0600)
	if err == nil {
		p.Upstreams = settings.Upstreams
		p.AppSelectors = settings.AppSelectors
		p.Debug = settings.Debug
	}
	p.Mu.Unlock()
	if err != nil {
		http.Error(w, "failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", "config-saved")
	w.WriteHeader(http.StatusNoContent)
}

func parseSettings(data []byte) (Settings, error) {
	var legacy []Upstream
	if err := yaml.Unmarshal(data, &legacy); err == nil {
		settings := Settings{Upstreams: legacy}
		if err := validateSettings(&settings); err != nil {
			return Settings{}, err
		}
		return settings, nil
	}
	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}
	if err := validateSettings(&settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
