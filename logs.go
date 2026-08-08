package agw

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type logHub struct {
	mu           sync.Mutex
	clients      map[chan string]struct{}
	history      []string
	historyLimit int
	file         *os.File
}

func newLogHub() *logHub {
	return &logHub{clients: make(map[chan string]struct{}), historyLimit: 100}
}

// newLogHubPersistent appends every log line to dir/logs.jsonl and restores
// the recent history from that file, so the request feed survives restarts.
func newLogHubPersistent(dir string) *logHub {
	hub := newLogHub()
	// The file keeps every line, so let a refreshed feed replay a much larger
	// slice of the persisted history instead of truncating to the in-memory cap.
	hub.historyLimit = 2000
	if err := os.MkdirAll(dir, 0755); err != nil {
		return hub
	}
	path := filepath.Join(dir, "logs.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return hub
	}
	hub.file = file
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > hub.historyLimit {
			lines = lines[len(lines)-hub.historyLimit:]
		}
		hub.history = lines
	}
	return hub
}

func (h *logHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.file != nil {
		_ = h.file.Close()
		h.file = nil
	}
}

func (h *logHub) Write(data []byte) (int, error) {
	message := strings.TrimRight(string(data), "\r\n")
	if message == "" {
		return len(data), nil
	}
	h.mu.Lock()
	if h.file != nil {
		_, _ = h.file.WriteString(message + "\n")
	}
	h.history = append(h.history, message)
	if len(h.history) > h.historyLimit {
		h.history = h.history[len(h.history)-h.historyLimit:]
	}
	for client := range h.clients {
		select {
		case client <- message:
		default:
		}
	}
	h.mu.Unlock()
	return len(data), nil
}

func (h *logHub) subscribe() (chan string, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	client := make(chan string, 32)
	h.clients[client] = struct{}{}
	history := append([]string(nil), h.history...)
	return client, history
}

func (h *logHub) unsubscribe(client chan string) {
	h.mu.Lock()
	delete(h.clients, client)
	close(client)
	h.mu.Unlock()
}

func writeSSE(w io.Writer, message string) error {
	for _, line := range strings.Split(message, "\n") {
		if _, err := io.WriteString(w, "data: "+line+"\n"); err != nil {
			return err
		}
	}
	// A second data line makes the EventSource payload end with a newline,
	// keeping successive log entries visually separated in the <pre> element.
	_, err := io.WriteString(w, "data: \n\n")
	return err
}
