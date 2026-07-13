package config

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"
)

// Duration is a YAML-friendly wrapper around time.Duration.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar, got YAML kind %d", node.Kind)
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", node.Value, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

func (d Duration) Value() time.Duration { return time.Duration(d) }
func (d Duration) String() string       { return time.Duration(d).String() }

// RuntimeConfig is immutable after Loader.Load returns successfully.
type RuntimeConfig struct {
	App           AppConfig           `yaml:"app"`
	Cognition     CognitionConfig     `yaml:"cognition"`
	Models        ModelsConfig        `yaml:"models"`
	Providers     ProvidersConfig     `yaml:"providers"`
	Storage       StorageConfig       `yaml:"storage"`
	Retrieval     RetrievalConfig     `yaml:"retrieval"`
	Emotion       EmotionConfig       `yaml:"emotion"`
	Observability ObservabilityConfig `yaml:"observability"`
	Limits        LimitsConfig        `yaml:"limits"`
	Features      FeaturesConfig      `yaml:"features"`

	profile     string
	loadedFiles []string
	secrets     map[string]SecretValue
}

func (c *RuntimeConfig) Profile() string { return c.profile }

func (c *RuntimeConfig) LoadedFiles() []string {
	return append([]string(nil), c.loadedFiles...)
}

func (c *RuntimeConfig) Secret(ref string) (SecretValue, bool) {
	value, ok := c.secrets[ref]
	return value, ok
}

type AppConfig struct {
	ServiceName     string               `yaml:"service_name"`
	Environment     string               `yaml:"environment"`
	HTTPAddress     string               `yaml:"http_address"`
	ShutdownTimeout Duration             `yaml:"shutdown_timeout"`
	OpenWebUI       OpenWebUITrustConfig `yaml:"openwebui"`
}

type OpenWebUITrustConfig struct {
	TrustForwardedHeaders   bool   `yaml:"trust_forwarded_headers"`
	InternalBearerSecretRef string `yaml:"internal_bearer_secret_ref"`
}

type CognitionConfig struct {
	DefaultRoute       string   `yaml:"default_route"`
	PhaseTimeout       Duration `yaml:"phase_timeout"`
	PrismConcurrency   int      `yaml:"prism_concurrency"`
	DegradedMinReports int      `yaml:"degraded_min_reports"`
	TokenBudget        int      `yaml:"token_budget"`
	ToolMaxCalls       int      `yaml:"tool_max_calls"`
}

type ModelsConfig struct {
	Allowlist map[string]ModelDefinition `yaml:"allowlist"`
	Roles     map[string]RoleModelConfig `yaml:"roles"`
}

type ModelDefinition struct {
	ProviderRef  string `yaml:"provider_ref"`
	Protocol     string `yaml:"protocol"`
	Enabled      bool   `yaml:"enabled"`
	PrivacyClass string `yaml:"privacy_class"`
	ReservedFor  string `yaml:"reserved_for,omitempty"`
}

type RoleModelConfig struct {
	Primary         string   `yaml:"primary"`
	Fallback        []string `yaml:"fallback"`
	Timeout         Duration `yaml:"timeout"`
	MaxRetries      int      `yaml:"max_retries"`
	Temperature     float64  `yaml:"temperature"`
	MaxOutputTokens int      `yaml:"max_output_tokens"`
}

type ProvidersConfig struct {
	OpenCodeZen EndpointConfig `yaml:"open_code_zen"`
	LangSearch  EndpointConfig `yaml:"langsearch"`
	Qdrant      EndpointConfig `yaml:"qdrant"`
	GrafanaOTLP EndpointConfig `yaml:"grafana_otlp"`
}

type EndpointConfig struct {
	Endpoint    string `yaml:"endpoint,omitempty"`
	EndpointRef string `yaml:"endpoint_ref,omitempty"`
	Protocol    string `yaml:"protocol"`
	APIKeyRef   string `yaml:"api_key_ref,omitempty"`
	HeadersRef  string `yaml:"headers_ref,omitempty"`
}

type StorageConfig struct {
	Database    DatabaseConfig    `yaml:"database"`
	River       RiverConfig       `yaml:"river"`
	ObjectStore ObjectStoreConfig `yaml:"object_store"`
}

type DatabaseConfig struct {
	URLRef          string   `yaml:"url_ref"`
	DirectURLRef    string   `yaml:"direct_url_ref"`
	MinConnections  int      `yaml:"min_connections"`
	MaxConnections  int      `yaml:"max_connections"`
	MaxConnLifetime Duration `yaml:"max_connection_lifetime"`
}

type RiverConfig struct {
	Enabled     bool `yaml:"enabled"`
	WorkerCount int  `yaml:"worker_count"`
}

type ObjectStoreConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Provider     string `yaml:"provider"`
	EndpointRef  string `yaml:"endpoint_ref,omitempty"`
	Bucket       string `yaml:"bucket,omitempty"`
	AccessKeyRef string `yaml:"access_key_ref,omitempty"`
	SecretKeyRef string `yaml:"secret_key_ref,omitempty"`
}

type RetrievalConfig struct {
	MemoryCollection   string        `yaml:"memory_collection"`
	DocumentCollection string        `yaml:"document_collection"`
	DenseModel         string        `yaml:"dense_model"`
	SparseModel        string        `yaml:"sparse_model"`
	TopK               int           `yaml:"top_k"`
	Fusion             string        `yaml:"fusion"`
	ColBERT            ColBERTConfig `yaml:"colbert"`
}

type ColBERTConfig struct {
	Enabled bool   `yaml:"enabled"`
	Model   string `yaml:"model"`
}

type EmotionConfig struct {
	Baseline     map[string]float64 `yaml:"baseline"`
	DecayRates   map[string]float64 `yaml:"decay_rates"`
	MaxDelta     float64            `yaml:"max_delta"`
	Relationship RelationshipConfig `yaml:"relationship"`
	Affective    AffectiveConfig    `yaml:"affective"`
}

type RelationshipConfig struct {
	PerUser bool `yaml:"per_user"`
	Global  bool `yaml:"global"`
}

type ObservabilityConfig struct {
	Logging LoggingConfig `yaml:"logging"`
	OTLP    OTLPConfig    `yaml:"otlp"`
}

type LoggingConfig struct {
	Level         string `yaml:"level"`
	Format        string `yaml:"format"`
	RedactContent bool   `yaml:"redact_content"`
}

type OTLPConfig struct {
	Enabled     bool    `yaml:"enabled"`
	ProviderRef string  `yaml:"provider_ref"`
	TraceSample float64 `yaml:"trace_sample"`
	Metrics     bool    `yaml:"metrics"`
	Logs        bool    `yaml:"logs"`
}

type LimitsConfig struct {
	RequestBytes        int64 `yaml:"request_bytes"`
	ManifestBytes       int64 `yaml:"manifest_bytes"`
	ContextTokens       int   `yaml:"context_tokens"`
	ActiveFullPipelines int   `yaml:"active_full_pipelines"`
	DocumentBytes       int64 `yaml:"document_bytes"`
}

type FeaturesConfig struct {
	Sandbox         bool `yaml:"sandbox"`
	ColBERT         bool `yaml:"colbert"`
	BYOKBridge      bool `yaml:"byok_bridge"`
	PublicAPI       bool `yaml:"public_api"`
	DocumentUploads bool `yaml:"document_uploads"`
}
