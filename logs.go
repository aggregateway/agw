package agw

import (
	"io"
	"strings"
	"sync"
)

type logHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
	history []string
}

func newLogHub() *logHub {
	return &logHub{clients: make(map[chan string]struct{})}
}

func (h *logHub) Write(data []byte) (int, error) {
	message := strings.TrimRight(string(data), "\r\n")
	if message == "" {
		return len(data), nil
	}
	h.mu.Lock()
	h.history = append(h.history, message)
	if len(h.history) > 100 {
		h.history = h.history[len(h.history)-100:]
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
