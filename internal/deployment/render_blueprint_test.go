package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	sonataServiceName    = "sonata-api"
	openWebUIServiceName = "sonata-web"
	openWebUIImage       = "ghcr.io/open-webui/open-webui:v0.10.2"
)

type blueprint struct {
	EnvVarGroups []envVarGroup `yaml:"envVarGroups"`
	Services     []service     `yaml:"services"`
}

type envVarGroup struct {
	Name    string   `yaml:"name"`
	EnvVars []envVar `yaml:"envVars"`
}

type service struct {
	Type                    string   `yaml:"type"`
	Name                    string   `yaml:"name"`
	Runtime                 string   `yaml:"runtime"`
	Region                  string   `yaml:"region"`
	Plan                    string   `yaml:"plan"`
	BuildCommand            string   `yaml:"buildCommand"`
	StartCommand            string   `yaml:"startCommand"`
	DockerCommand           string   `yaml:"dockerCommand"`
	HealthCheckPath         string   `yaml:"healthCheckPath"`
	MaxShutdownDelaySeconds int      `yaml:"maxShutdownDelaySeconds"`
	Image                   image    `yaml:"image"`
	Disk                    disk     `yaml:"disk"`
	EnvVars                 []envVar `yaml:"envVars"`
}

type image struct {
	URL string `yaml:"url"`
}

type disk struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	SizeGB    int    `yaml:"sizeGB"`
}

type envVar struct {
	Key           string       `yaml:"key"`
	Value         string       `yaml:"value"`
	Sync          *bool        `yaml:"sync"`
	GenerateValue bool         `yaml:"generateValue"`
	FromGroup     string       `yaml:"fromGroup"`
	FromService   *fromService `yaml:"fromService"`
}

type fromService struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	Property  string `yaml:"property"`
	EnvVarKey string `yaml:"envVarKey"`
}

func TestRenderBlueprintPreservesDeploymentBoundary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "render.yaml"))
	if err != nil {
		t.Fatalf("read render.yaml: %v", err)
	}
	var config blueprint
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode render.yaml: %v", err)
	}

	sonata := requireService(t, config.Services, sonataServiceName)
	if sonata.Type != "pserv" || sonata.Runtime != "go" {
		t.Fatalf("Sonata must remain a private Go service, got type=%q runtime=%q", sonata.Type, sonata.Runtime)
	}
	if sonata.HealthCheckPath != "/internal/health/ready" {
		t.Fatalf("unexpected Sonata health check path %q", sonata.HealthCheckPath)
	}
	if !strings.Contains(sonata.StartCommand, "sonata api") || !strings.Contains(sonata.StartCommand, "--profile production") {
		t.Fatalf("unexpected Sonata start command %q", sonata.StartCommand)
	}
	if sonata.MaxShutdownDelaySeconds < 20 {
		t.Fatalf("shutdown delay %d is shorter than application grace period", sonata.MaxShutdownDelaySeconds)
	}
	if envValue(sonata.EnvVars, "SONATA_PROFILE") != "production" {
		t.Fatal("Sonata production profile is not pinned")
	}
	if envByKey(sonata.EnvVars, "").FromGroup != "sonata-runtime-secrets" {
		t.Fatal("Sonata service does not consume the runtime secret group")
	}

	secrets := requireEnvGroup(t, config.EnvVarGroups, "sonata-runtime-secrets")
	for _, key := range []string{
		"OPENCODE_ZEN_API_KEY",
		"DATABASE_URL",
		"DATABASE_DIRECT_URL",
		"LANGSEARCH_API_KEY",
		"QDRANT_URL",
		"QDRANT_API_KEY",
		"GRAFANA_OTLP_ENDPOINT",
	} {
		item := envByKey(secrets.EnvVars, key)
		if item.Sync == nil || *item.Sync {
			t.Fatalf("secret %s must be supplied out of band", key)
		}
	}
	if !envByKey(secrets.EnvVars, "OPENWEBUI_INTERNAL_SECRET").GenerateValue {
		t.Fatal("OpenWebUI service credential must be generated, not committed")
	}

	web := requireService(t, config.Services, openWebUIServiceName)
	if web.Type != "web" || web.Runtime != "image" {
		t.Fatalf("OpenWebUI must remain a public image service, got type=%q runtime=%q", web.Type, web.Runtime)
	}
	if web.Image.URL != openWebUIImage || strings.HasSuffix(web.Image.URL, ":main") {
		t.Fatalf("OpenWebUI image must be version-pinned, got %q", web.Image.URL)
	}
	if web.HealthCheckPath != "/health" {
		t.Fatalf("unexpected OpenWebUI health check path %q", web.HealthCheckPath)
	}
	if !strings.Contains(web.DockerCommand, `OPENAI_API_BASE_URL="http://${SONATA_INTERNAL_ADDRESS}/v1"`) || !strings.Contains(web.DockerCommand, "exec bash start.sh") {
		t.Fatalf("OpenWebUI command does not bind its only OpenAI connection to Sonata: %q", web.DockerCommand)
	}
	if web.Disk.MountPath != "/app/backend/data" || web.Disk.SizeGB < 1 {
		t.Fatalf("OpenWebUI persistent data disk is invalid: %#v", web.Disk)
	}

	for key, want := range map[string]string{
		"ENABLE_OPENAI_API":                "true",
		"ENABLE_OLLAMA_API":                "false",
		"ENABLE_MEMORIES":                  "false",
		"ENABLE_FORWARD_USER_INFO_HEADERS": "true",
		"ANONYMIZED_TELEMETRY":             "false",
	} {
		if got := envValue(web.EnvVars, key); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}

	address := envByKey(web.EnvVars, "SONATA_INTERNAL_ADDRESS").FromService
	if address == nil || address.Name != sonataServiceName || address.Type != "pserv" || address.Property != "hostport" {
		t.Fatalf("OpenWebUI does not resolve Sonata through the private service address: %#v", address)
	}
	credential := envByKey(web.EnvVars, "OPENAI_API_KEY").FromService
	if credential == nil || credential.Name != sonataServiceName || credential.EnvVarKey != "OPENWEBUI_INTERNAL_SECRET" {
		t.Fatalf("OpenWebUI does not receive the Sonata service credential safely: %#v", credential)
	}

	for _, forbidden := range []string{
		"OPENAI_API_BASE_URLS",
		"OPENAI_API_KEYS",
		"OLLAMA_BASE_URL",
		"OLLAMA_BASE_URLS",
		"ANTHROPIC_API_KEY",
		"GOOGLE_API_KEY",
	} {
		if hasEnvKey(web.EnvVars, forbidden) {
			t.Fatalf("direct provider configuration %s must not be exposed to OpenWebUI", forbidden)
		}
	}
}

func requireService(t *testing.T, services []service, name string) service {
	t.Helper()
	for _, candidate := range services {
		if candidate.Name == name {
			return candidate
		}
	}
	t.Fatalf("service %q not found", name)
	return service{}
}

func requireEnvGroup(t *testing.T, groups []envVarGroup, name string) envVarGroup {
	t.Helper()
	for _, candidate := range groups {
		if candidate.Name == name {
			return candidate
		}
	}
	t.Fatalf("environment group %q not found", name)
	return envVarGroup{}
}

func envByKey(values []envVar, key string) envVar {
	for _, candidate := range values {
		if candidate.Key == key {
			return candidate
		}
	}
	return envVar{}
}

func envValue(values []envVar, key string) string {
	return envByKey(values, key).Value
}

func hasEnvKey(values []envVar, key string) bool {
	for _, candidate := range values {
		if candidate.Key == key {
			return true
		}
	}
	return false
}
