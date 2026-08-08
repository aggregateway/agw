package agw

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxSessionCards = 48
const maxRequestsPerSession = 8
const responsePreviewInterval = 150 * time.Millisecond

type sessionHeader struct {
	Name  string
	Value string
}

type sessionRequest struct {
	Sequence           uint64
	Method             string
	Path               string
	StartedAt          time.Time
	CompletedAt        time.Time
	Status             int
	State              string
	Bytes              int
	Headers            []sessionHeader
	ContentType        string
	LastPreview        time.Time
	RequestContentType string
	RequestBytes       int64
	ResponseBytes      int64
	RequestPath        string
	ResponsePath       string
	ResponseFile       *os.File
	AppSelector        string
	Upstream           string
	Model              string
	OriginalModel      string
	Events             []sessionEvent
}

type sessionEvent struct {
	At     time.Time
	Kind   string
	Detail string
}

type sessionRecord struct {
	ID           string
	FirstSeen    time.Time
	LastSeen     time.Time
	RequestCount int
	Requests     []*sessionRequest
}

type sessionHub struct {
	mu          sync.Mutex
	records     map[string]*sessionRecord
	subscribers map[chan struct{}]struct{}
	nextID      uint64
	payloadDir  string
}

type trackedSession struct {
	hub       *sessionHub
	sessionID string
	sequence  uint64
}

type sessionCard struct {
	ID           string
	ShortID      string
	Started      string
	Duration     string
	State        string
	StateClass   string
	Status       string
	RequestCount int
	AppSelector  string
	Upstream     string
	Latest       sessionRequestCard
	Requests     []sessionRequestCard
}

type sessionRequestCard struct {
	Method             string
	Path               string
	Started            string
	Duration           string
	State              string
	Status             string
	Bytes              string
	Headers            []sessionHeader
	ContentType        string
	RequestContentType string
	RequestBytes       string
	ResponseBytes      string
	AppSelector        string
	Upstream           string
	Model              string
	HasRequestBody     bool
	HasResponseBody    bool
	Events             []sessionEventCard
}

type sessionEventCard struct {
	At     string
	Kind   string
	Detail string
}

var sessionCardsTemplate = template.Must(template.New("session-cards").Parse(`{{range .}}
<article class="session-card" data-session-id="{{.ID}}">
  <button class="session-summary" type="button" data-session-toggle aria-expanded="false">
    <span class="session-indicator {{.StateClass}}"></span>
    <span class="session-primary"><span class="session-path"><b>{{.Latest.Method}}</b> {{.Latest.Path}}</span><span class="session-id">{{.ShortID}} · {{.Started}}</span></span>
    <span class="session-cell session-selector">{{if .Latest.AppSelector}}{{.Latest.AppSelector}}{{else}}<span class="session-empty-cell">—</span>{{end}}</span>
    <span class="session-cell session-upstream">{{if .Latest.Upstream}}{{.Latest.Upstream}}{{else}}<span class="session-empty-cell">—</span>{{end}}</span>
    <span class="session-cell session-model">{{if .Latest.Model}}{{.Latest.Model}}{{else}}<span class="session-empty-cell">—</span>{{end}}</span>
    <span class="session-state {{.StateClass}}">{{.State}}</span>
    <span class="session-metric session-status"><small>status</small><strong>{{.Status}}</strong></span>
    <span class="session-metric session-transfer"><small>latest transfer</small><strong>{{.Latest.Bytes}}</strong></span>
    <span class="session-metric session-duration"><small>duration</small><strong>{{.Duration}}</strong></span>
    <i data-lucide="chevron-down" class="session-chevron"></i>
  </button>
  <div class="session-details" hidden>
    <div class="session-overview"><span><small>App selector</small><strong>{{.Latest.AppSelector}}</strong></span><span><small>Model</small><strong>{{.Latest.Model}}</strong></span><span><small>Upstream</small><strong>{{.Latest.Upstream}}</strong></span><span><small>Connection</small><strong class="{{.StateClass}}">{{.State}}</strong></span><span><small>Requests</small><strong>{{.RequestCount}}</strong></span><span><small>Latest transfer</small><strong>{{.Latest.Bytes}}</strong></span><span><small>Latest duration</small><strong>{{.Latest.Duration}}</strong></span></div>
    <section class="session-headers"><h3>Latest request headers</h3><dl class="header-list">{{range .Latest.Headers}}<div><dt>{{.Name}}</dt><dd>{{.Value}}</dd></div>{{else}}<div><dt>Headers</dt><dd>unavailable</dd></div>{{end}}</dl></section>
    {{if or .Latest.HasRequestBody .Latest.HasResponseBody}}<div class="payload-open-buttons" data-payload-buttons>{{if .Latest.HasRequestBody}}<button class="payload-open" type="button" data-payload-open="request"><i data-lucide="arrow-up-right"></i><span>Intercepted request</span><small>{{.Latest.RequestContentType}} · {{.Latest.RequestBytes}}</small></button>{{end}}{{if .Latest.HasResponseBody}}<button class="payload-open" type="button" data-payload-open="response"><i data-lucide="arrow-down-left"></i><span>Intercepted response</span><small>{{.Latest.ContentType}} · {{.Latest.ResponseBytes}} · live tail</small></button>{{end}}</div>{{end}}
    <section class="session-events"><h3>Gateway events</h3><ol class="gateway-events">{{range .Latest.Events}}<li><time>{{.At}}</time><span class="gateway-event-kind">{{.Kind}}</span><span>{{.Detail}}</span></li>{{else}}<li class="gateway-events-empty">Waiting for upstream activity</li>{{end}}</ol></section>
  </div>
</article>{{else}}<div class="session-empty">暂无 API 会话。新的请求会实时出现在这里。</div>{{end}}`))

func newSessionHub() *sessionHub {
	directory, _ := os.MkdirTemp("", "agw-sessions-")
	return &sessionHub{records: make(map[string]*sessionRecord), subscribers: make(map[chan struct{}]struct{}), payloadDir: directory}
}

func (h *sessionHub) start(r *http.Request) *trackedSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	h.nextID++
	id := newSessionID(now, h.nextID)
	record := &sessionRecord{ID: id, FirstSeen: now}
	h.records[id] = record
	record.LastSeen = now
	record.RequestCount++
	request := &sessionRequest{Sequence: h.nextID, Method: r.Method, Path: r.URL.RequestURI(), StartedAt: now, State: "connecting", Headers: redactHeaders(r.Header)}
	if h.payloadDir != "" {
		request.RequestPath = filepath.Join(h.payloadDir, id+".request")
		request.ResponsePath = filepath.Join(h.payloadDir, id+".response")
		request.ResponseFile, _ = os.OpenFile(request.ResponsePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	}
	record.Requests = append(record.Requests, request)
	if len(record.Requests) > maxRequestsPerSession {
		pruned := record.Requests[:len(record.Requests)-maxRequestsPerSession]
		record.Requests = record.Requests[len(record.Requests)-maxRequestsPerSession:]
		removePayloadFiles(pruned)
	}
	h.publishLocked()
	h.evictLocked()
	return &trackedSession{hub: h, sessionID: id, sequence: request.Sequence}
}

// evictLocked drops the least recently seen records beyond maxSessionCards so
// the session map (and the on-disk payload files) cannot grow without bound.
func (h *sessionHub) evictLocked() {
	excess := len(h.records) - maxSessionCards
	for i := 0; i < excess; i++ {
		var oldestID string
		var oldest time.Time
		for id, record := range h.records {
			if oldestID == "" || record.LastSeen.Before(oldest) {
				oldestID = id
				oldest = record.LastSeen
			}
		}
		if oldestID == "" {
			return
		}
		record := h.records[oldestID]
		for _, request := range record.Requests {
			if request.ResponseFile != nil {
				_ = request.ResponseFile.Close()
			}
		}
		removePayloadFiles(record.Requests)
		delete(h.records, oldestID)
	}
}

func removePayloadFiles(requests []*sessionRequest) {
	for _, request := range requests {
		if request.RequestPath != "" {
			_ = os.Remove(request.RequestPath)
		}
		if request.ResponsePath != "" {
			_ = os.Remove(request.ResponsePath)
		}
	}
}

func (t *trackedSession) connected(status int) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.Status = status
		request.State = "streaming"
	})
}

func (t *trackedSession) setAppSelector(selector string) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.AppSelector = selector
	})
}

func (t *trackedSession) setUpstream(upstream string) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.Upstream = upstream
	})
}

func (t *trackedSession) setContentType(contentType string) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.ContentType = contentType
	})
}

func (t *trackedSession) setRequestBody(contentType string, body []byte) {
	path := t.hub.requestPath(t)
	var bytesWritten int64
	model := ""
	if len(body) > 0 && path != "" {
		if err := os.WriteFile(path, body, 0600); err == nil {
			bytesWritten = int64(len(body))
		}
	}
	if len(body) > 0 {
		model, _ = bodyFieldValue(body, "model")
	}
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.RequestContentType = contentType
		request.RequestBytes = bytesWritten
		request.Model = model
	})
}

func (t *trackedSession) setOriginalModel(model string) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.OriginalModel = model
	})
}

func (t *trackedSession) captureResponse(data []byte) {
	if len(data) == 0 {
		return
	}
	t.hub.captureResponse(t, data)
}

func (t *trackedSession) addEvent(kind, detail string) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.Events = append(request.Events, sessionEvent{At: time.Now(), Kind: kind, Detail: detail})
		if len(request.Events) > 24 {
			request.Events = request.Events[len(request.Events)-24:]
		}
	})
}

func (t *trackedSession) complete(status, bytes int, contextErr error) {
	t.hub.updateRequest(t, func(request *sessionRequest) {
		request.Status = status
		request.Bytes = bytes
		request.CompletedAt = time.Now()
		switch contextErr {
		case nil:
			request.State = "completed"
		case context.Canceled:
			request.State = "client closed"
		case context.DeadlineExceeded:
			request.State = "timed out"
		default:
			request.State = "interrupted"
		}
		if request.ResponseFile != nil {
			_ = request.ResponseFile.Close()
			request.ResponseFile = nil
		}
	})
}

func (h *sessionHub) updateRequest(tracked *trackedSession, update func(*sessionRequest)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	record := h.records[tracked.sessionID]
	if record == nil {
		return
	}
	for _, request := range record.Requests {
		if request.Sequence == tracked.sequence {
			update(request)
			record.LastSeen = time.Now()
			h.publishLocked()
			return
		}
	}
}

func (h *sessionHub) captureResponse(tracked *trackedSession, data []byte) {
	h.mu.Lock()
	record := h.records[tracked.sessionID]
	if record == nil {
		h.mu.Unlock()
		return
	}
	for _, request := range record.Requests {
		if request.Sequence != tracked.sequence {
			continue
		}
		request.Bytes += len(data)
		request.ResponseBytes += int64(len(data))
		record.LastSeen = time.Now()
		file := request.ResponseFile
		shouldPublish := request.LastPreview.IsZero() || time.Since(request.LastPreview) >= responsePreviewInterval
		if shouldPublish {
			request.LastPreview = time.Now()
		}
		h.mu.Unlock()
		if file != nil {
			_, _ = file.Write(data)
		}
		if shouldPublish {
			h.mu.Lock()
			h.publishLocked()
			h.mu.Unlock()
		}
		return
	}
	h.mu.Unlock()
}

func (h *sessionHub) cards() []sessionCard {
	h.mu.Lock()
	defer h.mu.Unlock()
	records := make([]*sessionRecord, 0, len(h.records))
	for _, record := range h.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].LastSeen.After(records[j].LastSeen) })
	if len(records) > maxSessionCards {
		records = records[:maxSessionCards]
	}
	cards := make([]sessionCard, 0, len(records))
	for _, record := range records {
		if len(record.Requests) == 0 {
			continue
		}
		latest := record.Requests[len(record.Requests)-1]
		requests := make([]sessionRequestCard, 0, len(record.Requests))
		for i := len(record.Requests) - 1; i >= 0; i-- {
			requests = append(requests, makeSessionRequestCard(record.Requests[i]))
		}
		state, class := sessionState(latest.State)
		cards = append(cards, sessionCard{ID: record.ID, ShortID: shortSessionID(record.ID), Started: record.FirstSeen.Format("15:04:05"), Duration: formatSessionDuration(record.FirstSeen, latest.CompletedAt), State: state, StateClass: class, Status: formatStatus(latest.Status), RequestCount: record.RequestCount, Latest: makeSessionRequestCard(latest), Requests: requests})
	}
	return cards
}

func (h *sessionHub) renderCards() (string, error) {
	var output bytes.Buffer
	if err := sessionCardsTemplate.Execute(&output, h.cards()); err != nil {
		return "", err
	}
	return output.String(), nil
}

func (h *sessionHub) close() {
	h.mu.Lock()
	for _, record := range h.records {
		for _, request := range record.Requests {
			if request.ResponseFile != nil {
				_ = request.ResponseFile.Close()
			}
		}
	}
	directory := h.payloadDir
	h.payloadDir = ""
	h.mu.Unlock()
	if directory != "" {
		_ = os.RemoveAll(directory)
	}
}

func (h *sessionHub) requestPath(tracked *trackedSession) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if request := h.findRequestLocked(tracked); request != nil {
		return request.RequestPath
	}
	return ""
}

func (h *sessionHub) readPayload(sessionID, kind string, tail int64) ([]byte, bool, error) {
	h.mu.Lock()
	record := h.records[sessionID]
	if record == nil || len(record.Requests) == 0 {
		h.mu.Unlock()
		return nil, false, nil
	}
	request := record.Requests[len(record.Requests)-1]
	path := request.RequestPath
	if kind == "response" {
		path = request.ResponsePath
	}
	h.mu.Unlock()
	if path == "" {
		return nil, false, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true, nil
	}
	if err != nil {
		return nil, true, err
	}
	defer file.Close()
	if tail > 0 {
		if info, err := file.Stat(); err == nil && info.Size() > tail {
			if _, err := file.Seek(-tail, io.SeekEnd); err != nil {
				return nil, true, err
			}
		}
	}
	data, err := io.ReadAll(file)
	return data, true, err
}

// payloadInfo resolves the on-disk payload file for a session and the content
// type recorded for it.
func (h *sessionHub) payloadInfo(sessionID, kind string) (path, contentType string, found bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	record := h.records[sessionID]
	if record == nil || len(record.Requests) == 0 {
		return "", "", false
	}
	request := record.Requests[len(record.Requests)-1]
	if kind == "response" {
		return request.ResponsePath, request.ContentType, request.ResponsePath != ""
	}
	return request.RequestPath, request.RequestContentType, request.RequestPath != ""
}

func (h *sessionHub) subscribe() chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	client := make(chan struct{}, 1)
	h.subscribers[client] = struct{}{}
	return client
}

func (h *sessionHub) unsubscribe(client chan struct{}) {
	h.mu.Lock()
	delete(h.subscribers, client)
	close(client)
	h.mu.Unlock()
}

func (h *sessionHub) publishLocked() {
	for client := range h.subscribers {
		select {
		case client <- struct{}{}:
		default:
		}
	}
}

func (h *sessionHub) findRequestLocked(tracked *trackedSession) *sessionRequest {
	record := h.records[tracked.sessionID]
	if record == nil {
		return nil
	}
	for _, request := range record.Requests {
		if request.Sequence == tracked.sequence {
			return request
		}
	}
	return nil
}

func (p *Proxy) serveSessions(w http.ResponseWriter, r *http.Request) {
	if p.Sessions == nil {
		http.Error(w, "session tracking is unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path == "/sessions" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		content, err := p.Sessions.renderCards()
		if err != nil {
			http.Error(w, "failed to render sessions", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, content)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	client := p.Sessions.subscribe()
	defer p.Sessions.unsubscribe(client)
	for {
		content, err := p.Sessions.renderCards()
		if err != nil || writeSessionSSE(w, content) != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-client:
		}
	}
}

func (p *Proxy) serveSessionPayload(w http.ResponseWriter, r *http.Request) {
	if p.Sessions == nil {
		http.Error(w, "session tracking is unavailable", http.StatusServiceUnavailable)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/sessions/"), "/")
	if len(parts) != 2 || (parts[1] != "request" && parts[1] != "response") {
		http.NotFound(w, r)
		return
	}
	// ?full=1 streams the complete on-disk payload instead of the preview
	// tail, so the whole response can be inspected without loading it on every
	// live refresh.
	if r.URL.Query().Get("full") == "1" {
		path, contentType, found := p.Sessions.payloadInfo(parts[0], parts[1])
		if !found {
			http.NotFound(w, r)
			return
		}
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, path)
		return
	}
	var tail int64
	if parts[1] == "response" {
		tail = 64 << 10
	}
	data, found, err := p.Sessions.readPayload(parts[0], parts[1], tail)
	if !found {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "failed to read session payload", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

func writeSessionSSE(w io.Writer, content string) error {
	if _, err := io.WriteString(w, "event: sessions\n"); err != nil {
		return err
	}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r", ""), "\n") {
		if _, err := io.WriteString(w, "data: "+line+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func newSessionID(now time.Time, sequence uint64) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		for i := range value {
			value[i] = byte(sequence >> (uint(i%8) * 8))
		}
	}
	milliseconds := uint64(now.UnixMilli())
	value[0] = byte(milliseconds >> 40)
	value[1] = byte(milliseconds >> 32)
	value[2] = byte(milliseconds >> 24)
	value[3] = byte(milliseconds >> 16)
	value[4] = byte(milliseconds >> 8)
	value[5] = byte(milliseconds)
	value[6] = 0x70 | value[6]&0x0f
	value[8] = 0x80 | value[8]&0x3f
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func redactHeaders(headers http.Header) []sessionHeader {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	output := make([]sessionHeader, 0, len(keys))
	for _, key := range keys {
		value := strings.Join(headers.Values(key), ", ")
		if isSensitiveHeader(key) {
			value = "[redacted]"
		}
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\r", " ")
		if len(value) > 220 {
			value = value[:217] + "..."
		}
		output = append(output, sessionHeader{Name: key, Value: value})
	}
	return output
}

func isSensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key":
		return true
	default:
		return false
	}
}

func makeSessionRequestCard(request *sessionRequest) sessionRequestCard {
	state, _ := sessionState(request.State)
	events := make([]sessionEventCard, 0, len(request.Events))
	for _, event := range request.Events {
		events = append(events, sessionEventCard{At: event.At.Format("15:04:05"), Kind: event.Kind, Detail: event.Detail})
	}
	model := request.Model
	if request.OriginalModel != "" && request.OriginalModel != request.Model {
		model = request.OriginalModel + " => " + request.Model
	}
	return sessionRequestCard{Method: request.Method, Path: request.Path, Started: request.StartedAt.Format("15:04:05"), Duration: formatSessionDuration(request.StartedAt, request.CompletedAt), State: state, Status: formatStatus(request.Status), Bytes: formatBytes(request.Bytes), Headers: request.Headers, ContentType: request.ContentType, RequestContentType: request.RequestContentType, RequestBytes: formatBytes64(request.RequestBytes), ResponseBytes: formatBytes64(request.ResponseBytes), AppSelector: request.AppSelector, Upstream: request.Upstream, Model: model, HasRequestBody: request.RequestBytes > 0, HasResponseBody: request.ResponseBytes > 0, Events: events}
}

func sessionState(state string) (string, string) {
	switch state {
	case "connecting":
		return "connecting", "is-connecting"
	case "streaming":
		return "streaming", "is-streaming"
	case "client closed":
		return "client closed", "is-warning"
	case "timed out", "interrupted":
		return state, "is-error"
	default:
		return "completed", "is-completed"
	}
}

func formatStatus(status int) string {
	if status == 0 {
		return "pending"
	}
	return fmt.Sprintf("%d", status)
}

func formatSessionDuration(start, end time.Time) string {
	if start.IsZero() {
		return "-"
	}
	if end.IsZero() {
		end = time.Now()
	}
	duration := end.Sub(start)
	if duration < time.Second {
		return duration.Round(time.Millisecond).String()
	}
	if duration < time.Minute {
		return duration.Round(time.Millisecond).String()
	}
	return duration.Round(time.Second).String()
}

func formatBytes(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func formatBytes64(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func shortSessionID(id string) string {
	if len(id) <= 18 {
		return id
	}
	return id[:8] + "..." + id[len(id)-6:]
}
