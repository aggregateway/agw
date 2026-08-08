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

	"gopkg.in/yaml.v3"
)

// Options configures the gateway when started programmatically or through the
// CLI.
type Options struct {
	ConfigPath string
	Listen     string
	Timeout    time.Duration
	// AllowDebug gates the config's debug flag: without it, debug header
	// logging stays off no matter what the config file or the client says.
	AllowDebug bool
	LogStderr  bool
}

// DefaultListenAddress derives the listen address from PORT, falling back to
// :8080. When PORT is set but not numeric, the offending value is returned so
// the caller can warn about it.
func DefaultListenAddress() (addr, invalidPort string) {
	addr = ":8080"
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if _, err := strconv.Atoi(port); err == nil {
			return ":" + port, ""
		}
		return addr, port
	}
	return addr, ""
}

// RunWithOptions starts the gateway and blocks until the HTTP server exits.
func RunWithOptions(opts Options) error {
	if opts.Timeout < 0 {
		return errors.New("timeout must be zero or greater")
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = "config.yaml"
	}
	if opts.Listen == "" {
		opts.Listen = ":8080"
	}
	adminUser := os.Getenv("AGW_ADMIN_USER")
	adminPassword := os.Getenv("AGW_ADMIN_PASSWORD")
	if (adminUser == "") != (adminPassword == "") {
		return errors.New("AGW_ADMIN_USER and AGW_ADMIN_PASSWORD must be set together")
	}

	settings, err := loadSettings(opts.ConfigPath)
	if err != nil {
		return err
	}
	// Secrets are injected by the admin browser via POST /config/secrets and
	// only ever live in memory. At startup, legacy literal values in
	// config.yaml are externalized in memory and the file is rewritten to
	// secret:<key> references so plaintext never stays on disk.
	upstreams, secretValues, migrated, err := externalizeSecrets(settings.Upstreams, map[string]string{})
	if err != nil {
		return err
	}
	hub := newLogHub()
	sessions := newSessionHub()
	defer sessions.close()
	logSink := io.Writer(hub)
	if opts.LogStderr {
		logSink = io.MultiWriter(os.Stderr, hub)
	}
	logger := slog.New(slog.NewJSONHandler(logSink, nil))
	client := newHTTPClient(opts.Timeout)
	if migrated {
		settings.Upstreams = upstreams
		encoded, marshalErr := yaml.Marshal(settings)
		if marshalErr != nil {
			return marshalErr
		}
		if writeErr := os.WriteFile(opts.ConfigPath, encoded, 0600); writeErr != nil {
			logger.Error("failed to rewrite config with secret references", "path", opts.ConfigPath, "error", writeErr.Error())
		} else {
			logger.Info("externalized auth values into memory; open the management UI to keep them in the browser", "config", opts.ConfigPath)
		}
	}
	proxy := &Proxy{Upstreams: upstreams, AppSelectors: settings.AppSelectors, Client: client, Logger: logger, Config: opts.ConfigPath, LogHub: hub, Sessions: sessions, AllowDebug: opts.AllowDebug, Debug: settings.Debug && opts.AllowDebug, SecretValues: secretValues}

	handler := requestLogger(logger, proxy)
	if adminUser != "" {
		handler = basicAuth(logger, adminUser, adminPassword, handler)
		logger.Info("management auth enabled", "username", adminUser)
	}
	server := &http.Server{
		Addr:              opts.Listen,
		Handler:           recoverJSON(logger, handler),
		ErrorLog:          slog.NewLogLogger(slog.NewJSONHandler(logSink, nil), slog.LevelError),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("server listening", "addr", opts.Listen, "upstreams", len(settings.Upstreams), "debug", proxy.Debug)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Run parses command-line arguments and starts the gateway. It exists for
// compatibility; the cobra CLI in cmd/agw uses RunWithOptions directly.
func Run(args []string) error {
	defaultAddr, invalidPort := DefaultListenAddress()
	opts := Options{ConfigPath: "config.yaml", Listen: defaultAddr}
	flags := flag.NewFlagSet("agw", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to upstream config")
	flags.StringVar(&opts.Listen, "listen", defaultAddr, "listen address")
	flags.DurationVar(&opts.Timeout, "timeout", 0, "per-upstream request timeout; 0 disables the timeout")
	flags.BoolVar(&opts.AllowDebug, "allow-debug", false, "allow client config (debug: true) to enable request header logging")
	flags.BoolVar(&opts.LogStderr, "log-stderr", false, "also write logs to stderr")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stdout)
			flags.Usage()
			return nil
		}
		return err
	}
	if invalidPort != "" {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("server config error", "port", invalidPort, "fallback", defaultAddr)
	}
	return RunWithOptions(opts)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	client := &http.Client{}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client
}
