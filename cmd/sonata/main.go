package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Lotargo/Sonata/internal/config"
	"github.com/Lotargo/Sonata/internal/httpapi"
	"go.yaml.in/yaml/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "api":
		return runAPI(ctx, args[1:], stderr)
	case "config":
		if len(args) < 2 {
			printConfigUsage(stderr)
			return 2
		}
		switch args[1] {
		case "validate":
			return runConfigValidate(ctx, args[2:], stdout, stderr)
		case "print":
			return runConfigPrint(ctx, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown config command %q\n", args[1])
			printConfigUsage(stderr)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "command %q is not implemented yet\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runAPI(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("api", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("config-root", "config", "configuration directory")
	profile := flags.String("profile", defaultProfile(), "configuration profile")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.NewLoader(nil).Load(ctx, *root, *profile)
	if err != nil {
		fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
		return 1
	}
	internalCredential, ok := cfg.Secret(cfg.App.OpenWebUI.InternalBearerSecretRef)
	if !ok || internalCredential.Empty() {
		fmt.Fprintln(stderr, "configuration invalid: OpenWebUI internal credential is unresolved")
		return 1
	}

	logger := newLogger(stderr, cfg.Observability.Logging.Level)
	handler := httpapi.NewHandler(httpapi.Options{
		Logger:             logger,
		RequestTimeout:     cfg.Cognition.PhaseTimeout.Value() * 6,
		MaxRequestBytes:    cfg.Limits.RequestBytes,
		InternalCredential: internalCredential.Reveal(),
	})
	server := httpapi.NewServer(cfg.App.HTTPAddress, handler, cfg.App.ShutdownTimeout.Value(), logger)
	logger.Info("starting Sonata API", "address", cfg.App.HTTPAddress, "profile", cfg.Profile())
	if err := server.Run(ctx); err != nil {
		logger.Error("Sonata API stopped with error", "error", err)
		return 1
	}
	logger.Info("Sonata API stopped")
	return 0
}

func newLogger(output io.Writer, configuredLevel string) *slog.Logger {
	level := slog.LevelInfo
	switch configuredLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
}

func runConfigValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("config-root", "config", "configuration directory")
	profile := flags.String("profile", defaultProfile(), "configuration profile")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.NewLoader(nil).Load(ctx, *root, *profile)
	if err != nil {
		fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "configuration valid: profile=%s files=%d\n", cfg.Profile(), len(cfg.LoadedFiles()))
	return 0
}

func runConfigPrint(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config print", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("config-root", "config", "configuration directory")
	profile := flags.String("profile", defaultProfile(), "configuration profile")
	redacted := flags.Bool("redacted", true, "print a redacted snapshot")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if !*redacted {
		fmt.Fprintln(stderr, "unredacted config output is forbidden")
		return 2
	}
	cfg, err := config.NewLoader(nil).Load(ctx, *root, *profile)
	if err != nil {
		fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
		return 1
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "marshal redacted configuration: %v\n", err)
		return 1
	}
	_, _ = stdout.Write(data)
	return 0
}

func defaultProfile() string {
	if value := os.Getenv("SONATA_PROFILE"); value != "" {
		return value
	}
	return "local"
}
func printUsage(w io.Writer) { fmt.Fprintln(w, "usage: sonata <api|config> [flags]") }
func printConfigUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  sonata config validate --config-root config --profile local")
	fmt.Fprintln(w, "  sonata config print --config-root config --profile local --redacted")
}
