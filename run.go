package agw

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Run starts the gateway and blocks until the HTTP server exits.
func Run(args []string) error {
	configPath := "config.yaml"
	listenDefault := ":8080"
	invalidPort := ""
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if _, err := strconv.Atoi(port); err == nil {
			listenDefault = ":" + port
		} else {
			invalidPort = port
		}
	}

	flags := flag.NewFlagSet("agw", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&configPath, "config", configPath, "path to upstream config")
	listen := flags.String("listen", listenDefault, "listen address")
	timeout := flags.Duration("timeout", 0, "per-upstream request timeout; 0 disables the timeout")
	debug := flags.Bool("debug", false, "log incoming request headers")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *timeout < 0 {
		return errors.New("timeout must be zero or greater")
	}

	settings, err := loadSettings(configPath)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if invalidPort != "" {
		logger.Error("server config error", "port", invalidPort, "fallback", listenDefault)
	}
	hub := newLogHub()
	sessions := newSessionHub()
	defer sessions.close()
	logger = slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stderr, hub), nil))
	client := newHTTPClient(*timeout)
	proxy := &Proxy{Upstreams: settings.Upstreams, AppSelectors: settings.AppSelectors, Client: client, Logger: logger, Config: configPath, LogHub: hub, Sessions: sessions, Debug: settings.Debug || *debug}

	server := &http.Server{
		Addr:              *listen,
		Handler:           recoverJSON(logger, requestLogger(logger, proxy)),
		ErrorLog:          slog.NewLogLogger(slog.NewJSONHandler(io.MultiWriter(os.Stderr, hub), nil), slog.LevelError),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("server listening", "addr", *listen, "upstreams", len(settings.Upstreams), "debug", proxy.Debug)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newHTTPClient(timeout time.Duration) *http.Client {
	client := &http.Client{}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client
}
