package agw

import (
	"errors"
	"flag"
	"io"
	"log"
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
	logger := log.New(os.Stderr, "[AGW] ", log.LstdFlags)
	if invalidPort != "" {
		logger.Printf("| SERVER | CONFIG_ERROR | invalid PORT %q, using %s", invalidPort, listenDefault)
	}
	hub := newLogHub()
	logger.SetOutput(io.MultiWriter(os.Stderr, hub))
	client := newHTTPClient(*timeout)
	proxy := &Proxy{Upstreams: settings.Upstreams, Client: client, Logger: logger, Config: configPath, LogHub: hub, Debug: settings.Debug || *debug}

	server := &http.Server{Addr: *listen, Handler: requestLogger(logger, proxy), ReadHeaderTimeout: 10 * time.Second}
	logger.Printf("| SERVER | LISTEN | addr=%s upstreams=%d debug=%t", *listen, len(settings.Upstreams), proxy.Debug)
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
