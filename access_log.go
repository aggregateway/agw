package agw

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

type accessWriter struct {
	http.ResponseWriter
	status        int
	bytes         int
	onWriteHeader func(int)
}

func (w *accessWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	if w.onWriteHeader != nil {
		w.onWriteHeader(status)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *accessWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (w *accessWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func requestLogger(logger Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		proxy, _ := next.(*Proxy)
		var session *trackedSession
		if proxy != nil && proxy.Sessions != nil && shouldTrackSession(r) {
			session = proxy.Sessions.start(r)
			r = r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, session))
		}
		if loggerProxyDebug(logger, next) {
			logger.Printf("| REQUEST | HEADERS | %s", formatHeaders(r.Header))
		}
		writer := &accessWriter{ResponseWriter: w}
		if session != nil {
			writer.onWriteHeader = session.connected
		}
		next.ServeHTTP(writer, r)
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		if session != nil {
			session.complete(status, writer.bytes, r.Context().Err())
		}
		logger.Printf("| %3d | %13v | %15s | %-7s %s | %dB", status, time.Since(started), clientIP(r), r.Method, r.URL.RequestURI(), writer.bytes)
	})
}

func shouldTrackSession(r *http.Request) bool {
	if isManagementRequest(r) {
		return false
	}
	return r.Header.Get("Session-Id") != "" || r.Header.Get("Thread-Id") != "" || strings.HasPrefix(r.URL.Path, "/v1/")
}

func loggerProxyDebug(logger Logger, next http.Handler) bool {
	proxy, ok := next.(*Proxy)
	if !ok {
		return false
	}
	proxy.Mu.RLock()
	debug := proxy.Debug
	proxy.Mu.RUnlock()
	return debug
}

func formatHeaders(headers http.Header) string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, headers.Values(key)))
	}
	return strings.Join(parts, " ")
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return fmt.Sprintf("%s", r.RemoteAddr)
}

type Logger interface {
	Printf(format string, args ...any)
}
