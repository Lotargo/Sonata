package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Lotargo/Sonata/internal/application"
	"github.com/Lotargo/Sonata/internal/config"
	"github.com/Lotargo/Sonata/internal/database"
	"github.com/Lotargo/Sonata/internal/emotion"
	"github.com/Lotargo/Sonata/internal/httpapi"
	"github.com/Lotargo/Sonata/internal/protected"
	"github.com/Lotargo/Sonata/internal/provider"
	"github.com/jackc/pgx/v5/pgxpool"
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

	// 1. Resolve DB Connection String & Construct pgxpool
	dbConnStr, ok := cfg.Secret(cfg.Storage.Database.URLRef)
	if !ok || dbConnStr.Empty() {
		logger.Error("database connection string is unresolved")
		return 1
	}
	pool, err := pgxpool.New(ctx, dbConnStr.Reveal())
	if err != nil {
		logger.Error("connect to database pool failed", "error", err)
		return 1
	}
	defer pool.Close()

	// 2. Initialize database repositories
	maxManifestBytes := int(cfg.Limits.ManifestBytes)
	if maxManifestBytes <= 0 {
		maxManifestBytes = protected.DefaultMaxUserManifestBytes
	}

	runRepo, err := database.NewRunRepository(pool)
	if err != nil {
		logger.Error("create run repository failed", "error", err)
		return 1
	}
	manifestRepo, err := database.NewManifestRepository(pool, maxManifestBytes)
	if err != nil {
		logger.Error("create manifest repository failed", "error", err)
		return 1
	}
	instructionRepo, err := database.NewInstructionRepository(pool)
	if err != nil {
		logger.Error("create instruction repository failed", "error", err)
		return 1
	}
	providerUsageRepo, err := database.NewProviderUsageRepository(pool)
	if err != nil {
		logger.Error("create provider usage repository failed", "error", err)
		return 1
	}
	affectiveStore, err := database.NewPostgresAffectiveStateStore(pool)
	if err != nil {
		logger.Error("create affective state store failed", "error", err)
		return 1
	}

	// 3. Load protected bundle & sync instruction versions
	bundle, err := protected.Load(os.DirFS("protected"), "registry.json")
	if err != nil {
		logger.Error("load protected bundle failed", "error", err)
		return 1
	}
	for id, inst := range bundle.Instructions {
		metaBytes, _ := json.Marshal(inst)
		_, err := instructionRepo.Upsert(ctx, database.UpsertInstructionVersionInput{
			InstructionID: id,
			Version:       int32(inst.Version),
			ContentHash:   inst.Hash,
			Metadata:      metaBytes,
			CreatedAt:     time.Now(),
		})
		if err != nil {
			logger.Error("sync instruction version failed", "instruction_id", id, "error", err)
			return 1
		}
	}
	logger.Info("synchronized protected instructions to database")

	// 4. Construct PromptCompiler & ManifestResolver
	compiler, err := protected.NewPromptCompiler(bundle)
	if err != nil {
		logger.Error("create prompt compiler failed", "error", err)
		return 1
	}
	resolver, err := protected.NewManifestResolver(bundle, maxManifestBytes)
	if err != nil {
		logger.Error("create manifest resolver failed", "error", err)
		return 1
	}

	// 5. Construct Provider and Model Router
	provCfg := cfg.Providers.OpenCodeZen
	provCred, ok := cfg.Secret(provCfg.APIKeyRef)
	if !ok || provCred.Empty() {
		logger.Error("open_code_zen credential unresolved")
		return 1
	}
	prov, err := provider.NewOpenCodeZenProviderFromConfig(provCfg, provCred, http.DefaultClient, cfg.Models.Allowlist)
	if err != nil {
		logger.Error("create open_code_zen provider failed", "error", err)
		return 1
	}
	router, err := provider.NewModelRouter(prov, cfg.Models.Roles, cfg.Models.Allowlist, provider.RouterOptions{})
	if err != nil {
		logger.Error("create model router failed", "error", err)
		return 1
	}

	// 6. Build Runner Adapter
	runners, err := application.NewRunnerAdapter(router, compiler, bundle, providerUsageRepo)
	if err != nil {
		logger.Error("create runners adapter failed", "error", err)
		return 1
	}

	// 7. Initialize CognitiveChatServiceImpl
	cognitiveChat, err := application.NewCognitiveChatServiceImpl(
		runners,
		runRepo,
		manifestRepo,
		bundle,
		resolver,
	)
	if err != nil {
		logger.Error("create cognitive chat service failed", "error", err)
		return 1
	}

	// 8. Initialize AffectiveRuntime & AffectiveChatService
	affectiveProfile, err := emotion.NewAffectiveRuntimeProfileFromConfig(cfg.Emotion)
	if err != nil {
		logger.Error("build affective runtime profile failed", "error", err)
		return 1
	}
	affectiveRuntime, err := emotion.NewAffectiveRuntime("sonata", affectiveProfile, affectiveStore, time.Now)
	if err != nil {
		logger.Error("create affective runtime failed", "error", err)
		return 1
	}
	affectiveChat, err := application.NewAffectiveChatService(affectiveRuntime, cognitiveChat)
	if err != nil {
		logger.Error("create affective chat service failed", "error", err)
		return 1
	}

	ready := func(ctx context.Context) error {
		return pool.Ping(ctx)
	}

	handler := httpapi.NewHandler(httpapi.Options{
		Logger:             logger,
		RequestTimeout:     cfg.Cognition.PhaseTimeout.Value() * 6,
		MaxRequestBytes:    cfg.Limits.RequestBytes,
		Ready:              ready,
		InternalCredential: internalCredential.Reveal(),
		Chat:               affectiveChat,
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
