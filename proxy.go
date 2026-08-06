package agw

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Authorization struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

type Upstream struct {
	URL           string         `yaml:"url"`
	Authorization *Authorization `yaml:"authorization"`
}

type Proxy struct {
	Upstreams []Upstream
	Client    *http.Client
	Logger    *log.Logger
	Config    string
	LogHub    *logHub
	Mu        sync.RWMutex
}

func loadConfig(path string) ([]Upstream, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var upstreams []Upstream
	if err := yaml.Unmarshal(data, &upstreams); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := validateUpstreams(upstreams); err != nil {
		return nil, err
	}
	return upstreams, nil
}

func validateUpstreams(upstreams []Upstream) error {
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
	p.Mu.RUnlock()

	if r.URL.Path == "/" && r.Method == http.MethodGet {
		serveConfigPage(w, r, upstreams)
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

	var lastErr error
	for i, upstream := range upstreams {
		p.Logger.Printf("upstream[%d] attempting %s %s", i, r.Method, r.URL.RequestURI())
		header, err := authorizationHeader(upstream.Authorization)
		if err != nil {
			lastErr = err
			p.Logger.Printf("upstream[%d] configuration error: %v", i, err)
			continue
		}
		target, err := upstreamRequestURL(upstream.URL, r.URL.RequestURI())
		if err != nil {
			lastErr = err
			p.Logger.Printf("upstream[%d] invalid target: %v", i, err)
			continue
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			p.Logger.Printf("upstream[%d] request creation failed: %v", i, err)
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
			p.Logger.Printf("upstream[%d] transport error: %v", i, err)
			continue
		}
		if resp.StatusCode >= http.StatusBadRequest {
			errorBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			}
			p.Logger.Printf("upstream[%d] returned %s for %s %s: %s", i, resp.Status, r.Method, target, strings.TrimSpace(string(errorBody)))
			if retryableStatus(resp.StatusCode) && i < len(upstreams)-1 {
				p.Logger.Printf("upstream[%d] retryable status, trying upstream[%d]", i, i+1)
				continue
			}
			copyResponseHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(errorBody)
			return
		}
		p.Logger.Printf("upstream[%d] returned %s, using response", i, resp.Status)
		copyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		resp.Body.Close()
		return
	}

	if r.Context().Err() != nil {
		return
	}
	p.Logger.Printf("all upstreams exhausted; last error: %v", lastErr)
	http.Error(w, "all upstreams failed", http.StatusBadGateway)
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
	var upstreams []Upstream
	if err := yaml.Unmarshal(data, &upstreams); err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateUpstreams(upstreams); err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusBadRequest)
		return
	}
	encoded, err := yaml.Marshal(upstreams)
	if err != nil {
		http.Error(w, "failed to encode config", http.StatusInternalServerError)
		return
	}
	p.Mu.Lock()
	err = os.WriteFile(p.Config, encoded, 0600)
	if err == nil {
		p.Upstreams = upstreams
	}
	p.Mu.Unlock()
	if err != nil {
		http.Error(w, "failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", "config-saved")
	w.WriteHeader(http.StatusNoContent)
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
