package agw

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
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
	Enabled       *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	regex         *regexp.Regexp
}

type BodyMatch struct {
	Field         string `yaml:"field" json:"field"`
	Operator      string `yaml:"operator" json:"operator"`
	Value         string `yaml:"value,omitempty" json:"value,omitempty"`
	CaseSensitive bool   `yaml:"caseSensitive,omitempty" json:"caseSensitive,omitempty"`
	Enabled       *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	regex         *regexp.Regexp
}

type PathMatch struct {
	Operator string `yaml:"operator" json:"operator"`
	Value    string `yaml:"value,omitempty" json:"value,omitempty"`
	Enabled  *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	regex    *regexp.Regexp
}

type QueryMatch struct {
	Name          string `yaml:"name" json:"name"`
	Operator      string `yaml:"operator" json:"operator"`
	Value         string `yaml:"value" json:"value"`
	CaseSensitive bool   `yaml:"caseSensitive,omitempty" json:"caseSensitive,omitempty"`
	Enabled       *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	regex         *regexp.Regexp
}

type AppSelector struct {
	Name    string           `yaml:"name" json:"name"`
	Match   AppSelectorMatch `yaml:"match,omitempty" json:"match,omitempty"`
	Rewrite []FieldRewrite   `yaml:"rewrite,omitempty" json:"rewrite,omitempty"`
}

type AppSelectorMatch struct {
	Path    []PathMatch   `yaml:"path,omitempty" json:"path,omitempty"`
	Query   []QueryMatch  `yaml:"query,omitempty" json:"query,omitempty"`
	Headers []HeaderMatch `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body    []BodyMatch   `yaml:"body,omitempty" json:"body,omitempty"`
}

type FieldRewrite struct {
	Field   string `yaml:"field" json:"field"`
	Value   string `yaml:"value" json:"value"`
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// RuleEnabled reports whether a rule is active. Rules without an explicit
// "enabled" field default to enabled so existing configs keep working.
func (m HeaderMatch) RuleEnabled() bool  { return m.Enabled == nil || *m.Enabled }
func (m BodyMatch) RuleEnabled() bool    { return m.Enabled == nil || *m.Enabled }
func (m PathMatch) RuleEnabled() bool    { return m.Enabled == nil || *m.Enabled }
func (m QueryMatch) RuleEnabled() bool   { return m.Enabled == nil || *m.Enabled }
func (r FieldRewrite) RuleEnabled() bool { return r.Enabled == nil || *r.Enabled }

// HasRules reports whether a selector defines any rules (enabled or disabled).
func (s AppSelector) HasRules() bool {
	return len(s.Match.Path)+len(s.Match.Query)+len(s.Match.Headers)+len(s.Match.Body)+len(s.Rewrite) > 0
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
			if !matcher.RuleEnabled() {
				continue
			}
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
		for q := range selector.Match.Query {
			matcher := &selector.Match.Query[q]
			if !matcher.RuleEnabled() {
				continue
			}
			if strings.TrimSpace(matcher.Name) == "" || strings.ContainsAny(matcher.Name, "\r\n") || strings.ContainsAny(matcher.Value, "\r\n") {
				return fmt.Errorf("app selector %q query rule %d is invalid", name, q+1)
			}
			if !supportedHeaderOperator(matcher.Operator) {
				return fmt.Errorf("app selector %q has unsupported query operator %q", name, matcher.Operator)
			}
			if err := compileQueryMatcher(matcher); err != nil {
				return fmt.Errorf("app selector %q query rule %d: %w", name, q+1, err)
			}
		}
		for k := range selector.Match.Body {
			matcher := &selector.Match.Body[k]
			if !matcher.RuleEnabled() {
				continue
			}
			if strings.TrimSpace(matcher.Field) == "" || strings.ContainsAny(matcher.Field, "\r\n") || strings.ContainsAny(matcher.Value, "\r\n") {
				return fmt.Errorf("app selector %q body rule %d is invalid", name, k+1)
			}
			if !supportedHeaderOperator(matcher.Operator) {
				return fmt.Errorf("app selector %q has unsupported body operator %q", name, matcher.Operator)
			}
			if err := compileBodyMatcher(matcher); err != nil {
				return fmt.Errorf("app selector %q body rule %d: %w", name, k+1, err)
			}
		}
		for r := range selector.Rewrite {
			rewrite := &selector.Rewrite[r]
			if !rewrite.RuleEnabled() {
				continue
			}
			if strings.TrimSpace(rewrite.Field) == "" || strings.ContainsAny(rewrite.Field, "\r\n") || strings.ContainsAny(rewrite.Value, "\r\n") {
				return fmt.Errorf("app selector %q rewrite rule %d is invalid", name, r+1)
			}
		}
		for p := range selector.Match.Path {
			path := &selector.Match.Path[p]
			if !path.RuleEnabled() {
				continue
			}
			if !supportedHeaderOperator(path.Operator) {
				return fmt.Errorf("app selector %q has unsupported path operator %q", name, path.Operator)
			}
			if !strings.EqualFold(strings.TrimSpace(path.Operator), "present") && strings.TrimSpace(path.Value) == "" {
				return fmt.Errorf("app selector %q path rule requires a non-empty value", name)
			}
			if err := compilePathMatcher(path); err != nil {
				return fmt.Errorf("app selector %q path rule %d: %w", name, p+1, err)
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

func compileQueryMatcher(matcher *QueryMatch) error {
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

func compileBodyMatcher(matcher *BodyMatch) error {
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

func compilePathMatcher(matcher *PathMatch) error {
	matcher.regex = nil
	if !strings.EqualFold(matcher.Operator, "regex") {
		return nil
	}
	if matcher.Value == "" {
		return errors.New("regex value is empty")
	}
	compiled, err := regexp.Compile(matcher.Value)
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

// queryMatchMatches matches URL query parameters (r.URL.Query()). The parameter
// name lookup is exact, values follow the same operator semantics as headers,
// and by default comparison is case-insensitive unless caseSensitive is set.
func queryMatchMatches(matcher QueryMatch, query url.Values) bool {
	values := query[matcher.Name]
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
				if err := compileQueryMatcher(&matcher); err != nil {
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

func appSelectorMatches(selector AppSelector, path string, query url.Values, headers http.Header, body []byte) bool {
	for _, matcher := range selector.Match.Path {
		if !matcher.RuleEnabled() {
			continue
		}
		if !pathMatchMatches(matcher, path) {
			return false
		}
	}
	for _, matcher := range selector.Match.Query {
		if !matcher.RuleEnabled() {
			continue
		}
		if !queryMatchMatches(matcher, query) {
			return false
		}
	}
	for _, matcher := range selector.Match.Headers {
		if !matcher.RuleEnabled() {
			continue
		}
		if !headerMatchMatches(matcher, headers) {
			return false
		}
	}
	for _, matcher := range selector.Match.Body {
		if !matcher.RuleEnabled() {
			continue
		}
		if !bodyMatchMatches(matcher, body) {
			return false
		}
	}
	return true
}

// pathMatchMatches matches the request URL path (without query string). Path
// rules are case-sensitive, unlike header/body rules, because URL paths are
// case-sensitive by spec; the operator set is the same as headers.
func pathMatchMatches(matcher PathMatch, path string) bool {
	if strings.EqualFold(matcher.Operator, "present") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(matcher.Operator)) {
	case "exact":
		return path == matcher.Value
	case "prefix":
		return strings.HasPrefix(path, matcher.Value)
	case "contains":
		return strings.Contains(path, matcher.Value)
	case "regex":
		compiled := matcher.regex
		if compiled == nil {
			if err := compilePathMatcher(&matcher); err != nil {
				return false
			}
			compiled = matcher.regex
		}
		return compiled.MatchString(path)
	}
	return false
}

func bodyMatchMatches(matcher BodyMatch, body []byte) bool {
	value, ok := bodyFieldValue(body, matcher.Field)
	if !ok {
		return false
	}
	if strings.EqualFold(matcher.Operator, "present") {
		return true
	}
	needle := matcher.Value
	comparison := value
	if !matcher.CaseSensitive {
		needle = strings.ToLower(needle)
		comparison = strings.ToLower(comparison)
	}
	switch strings.ToLower(strings.TrimSpace(matcher.Operator)) {
	case "exact":
		return comparison == needle
	case "prefix":
		return strings.HasPrefix(comparison, needle)
	case "contains":
		return strings.Contains(comparison, needle)
	case "regex":
		compiled := matcher.regex
		if compiled == nil {
			if err := compileBodyMatcher(&matcher); err != nil {
				return false
			}
			compiled = matcher.regex
		}
		return compiled.MatchString(value)
	}
	return false
}

// bodyFieldValue extracts the JSON value at a dotted field path (for example
// "model" or "metadata.provider"). Scalar values are converted to their string
// form; arrays and objects are JSON-encoded so contains/exact can search their
// serialized text. Non-JSON bodies and missing fields never match.
func bodyFieldValue(body []byte, field string) (string, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var doc any
	if err := decoder.Decode(&doc); err != nil {
		return "", false
	}
	current := doc
	for _, part := range strings.Split(field, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", false
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = obj[part]
		if !ok {
			return "", false
		}
	}
	if current == nil {
		return "", false
	}
	return scalarString(current), true
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return ""
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

// applyRewrites sets JSON body fields with jq-style setpath semantics:
// intermediate objects are created when missing and typed values (numbers,
// booleans, null, arrays, objects) keep their JSON type. Only object-rooted
// JSON bodies are rewritten; everything else is returned unchanged. The
// original key order of every object is preserved.
func applyRewrites(body []byte, rewrites []FieldRewrite) []byte {
	if len(rewrites) == 0 {
		return body
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return body
	}
	doc, err := parseOrderedJSON(trimmed)
	if err != nil {
		return body
	}
	root, ok := doc.(*orderedObject)
	if !ok {
		return body
	}
	for _, rewrite := range rewrites {
		if !rewrite.RuleEnabled() {
			continue
		}
		setJSONPath(root, rewrite.Field, parseRewriteValue(rewrite.Value))
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return encoded
}

// orderedObject keeps object keys in their original JSON order while still
// allowing fast lookup by key.
type orderedObject struct {
	keys   []string
	values map[string]any
}

func (o *orderedObject) set(key string, value any) {
	if o.values == nil {
		o.values = make(map[string]any)
	}
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			output.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		output.Write(encodedKey)
		output.WriteByte(':')
		encodedValue, err := json.Marshal(o.values[key])
		if err != nil {
			return nil, err
		}
		output.Write(encodedValue)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

// parseOrderedJSON decodes JSON into a tree of orderedObject / []any / scalar
// nodes, preserving the key order of every object. Numbers keep their original
// literal via json.Number.
func parseOrderedJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decodeJSONValue(decoder)
}

func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := &orderedObject{values: make(map[string]any)}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object.set(key, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		array := []any{}
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return array, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

func setJSONPath(root *orderedObject, field string, value any) {
	parts := strings.Split(field, ".")
	if len(parts) == 0 {
		return
	}
	current := root
	for _, part := range parts[:len(parts)-1] {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		next, ok := current.values[part].(*orderedObject)
		if !ok {
			next = &orderedObject{values: make(map[string]any)}
			current.set(part, next)
		}
		current = next
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last != "" {
		current.set(last, value)
	}
}

// parseRewriteValue treats the configured value as JSON when it parses, so
// numbers, booleans, null, arrays and objects keep their JSON type; otherwise
// the raw text is used as a plain string.
func parseRewriteValue(value string) any {
	trimmed := strings.TrimSpace(value)
	if parsed, err := parseOrderedJSON([]byte(trimmed)); err == nil {
		return parsed
	}
	return value
}

func applySelectorRewrites(body []byte, selectors []AppSelector, selected string, logger *log.Logger, session *trackedSession) []byte {
	for _, selector := range selectors {
		if selector.Name != selected || len(selector.Rewrite) == 0 {
			continue
		}
		rewritten := applyRewrites(body, selector.Rewrite)
		if bytes.Equal(rewritten, body) {
			return body
		}
		for _, rewrite := range selector.Rewrite {
			detail := rewrite.Field + " -> " + rewrite.Value
			logger.Printf("| REWRITE | %s", detail)
			if session != nil {
				session.addEvent("rewrite", detail)
			}
		}
		return rewritten
	}
	return body
}

func upstreamSupportsSelector(upstream Upstream, selector string) bool {
	for _, name := range upstream.AppSelectors {
		if name == selector {
			return true
		}
	}
	return false
}

func routeUpstreams(upstreams []Upstream, selectors []AppSelector, path string, query url.Values, headers http.Header, body []byte) ([]routedUpstream, string, error) {
	if len(selectors) == 0 {
		routed := make([]routedUpstream, 0, len(upstreams))
		for index, upstream := range upstreams {
			routed = append(routed, routedUpstream{Index: index, Upstream: upstream})
		}
		return routed, "", nil
	}
	for _, selector := range selectors {
		if !appSelectorMatches(selector, path, query, headers, body) {
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
	originalModel := ""
	if session != nil {
		originalModel, _ = bodyFieldValue(body, "model")
	}
	routedUpstreams, appSelector, routeErr := routeUpstreams(upstreams, appSelectors, r.URL.Path, r.URL.Query(), r.Header, body)
	if routeErr != nil {
		p.Logger.Printf("| ROUTER | NO_MATCH | %v", routeErr)
		if session != nil {
			session.setRequestBody(r.Header.Get("Content-Type"), body)
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
		body = applySelectorRewrites(body, appSelectors, appSelector, p.Logger, session)
	}
	if session != nil {
		session.setRequestBody(r.Header.Get("Content-Type"), body)
		session.setOriginalModel(originalModel)
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
