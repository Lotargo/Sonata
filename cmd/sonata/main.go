package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Lotargo/Sonata/internal/config"
	"go.yaml.in/yaml/v3"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	if args[0] != "config" {
		fmt.Fprintf(stderr, "command %q is not implemented yet\n", args[0])
		printUsage(stderr)
		return 2
	}
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

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: sonata config <validate|print> [flags]")
}

func printConfigUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  sonata config validate --config-root config --profile local")
	fmt.Fprintln(w, "  sonata config print --config-root config --profile local --redacted")
}
