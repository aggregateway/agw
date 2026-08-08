package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/aggregateway/agw"
	"github.com/spf13/cobra"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	defaultAddr, invalidPort := agw.DefaultListenAddress()
	opts := agw.Options{ConfigPath: "config.yaml", Listen: defaultAddr}
	root := newRootCommand(&opts, defaultAddr, invalidPort, logger)
	if err := root.Execute(); err != nil {
		logger.Error("fatal", "error", err.Error())
		os.Exit(1)
	}
}

func newRootCommand(opts *agw.Options, defaultAddr, invalidPort string, logger *slog.Logger) *cobra.Command {
	root := &cobra.Command{
		Use:   "agw",
		Short: "Configured HTTP reverse proxy for AI APIs",
		Long: "agw routes API requests to configured upstreams with routing\n" +
			"selectors, body rewriting and a management UI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if invalidPort != "" {
				logger.Error("server config error", "port", invalidPort, "fallback", defaultAddr)
			}
			return agw.RunWithOptions(*opts)
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	flags := root.Flags()
	flags.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to upstream config")
	flags.StringVar(&opts.Listen, "listen", defaultAddr, "listen address (defaults to $PORT or :8080)")
	flags.DurationVar(&opts.Timeout, "timeout", 0, "per-upstream request timeout; 0 disables the timeout")
	flags.BoolVar(&opts.AllowDebug, "allow-debug", false, "allow client config (debug: true) to enable request header logging")
	flags.BoolVar(&opts.LogStderr, "log-stderr", false, "also write logs to stderr (default: only the /logs feed)")
	// Keep accepting single-dash long flags (-config) as the original CLI did;
	// pflag would otherwise read them as shorthand clusters.
	root.SetArgs(normalizeArgs(os.Args[1:]))
	return root
}

// normalizeArgs rewrites known long flags written with a single dash into
// double-dash form so Go-style invocations like -config keep working.
func normalizeArgs(args []string) []string {
	longFlags := map[string]bool{"config": true, "listen": true, "timeout": true, "allow-debug": true, "log-stderr": true}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(strings.SplitN(arg, "=", 2)[0], "-")
			if longFlags[name] {
				arg = "-" + arg
			}
		}
		out = append(out, arg)
	}
	return out
}
