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
	timeout := flags.Duration("timeout", 60*time.Second, "per-upstream request timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	upstreams, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	logger := log.New(os.Stderr, "agw: ", log.LstdFlags)
	if invalidPort != "" {
		logger.Printf("invalid PORT %q, using %s", invalidPort, listenDefault)
	}
	hub := newLogHub()
	logger.SetOutput(io.MultiWriter(os.Stderr, hub))
	client := &http.Client{Timeout: *timeout}
	proxy := &Proxy{Upstreams: upstreams, Client: client, Logger: logger, Config: configPath, LogHub: hub}

	server := &http.Server{Addr: *listen, Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	logger.Printf("listening on %s with %d upstreams", *listen, len(upstreams))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
