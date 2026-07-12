package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

var requiredModelRoles = []string{
	"router",
	"raw",
	"critical",
	"summary",
	"synthesis_tooling",
	"synthesis_final",
}

func (c *RuntimeConfig) Validate(profile string) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if c.App.ServiceName == "" {
		add("app.service_name is required")
	}
	if c.App.Environment != profile {
		add("app.environment %q must match selected profile %q", c.App.Environment, profile)
	}
	if c.App.HTTPAddress == "" {
		add("app.http_address is required")
	}
	if c.App.ShutdownTimeout.Value() <= 0 {
		add("app.shutdown_timeout must be positive")
	}
	if c.App.OpenWebUI.TrustForwardedHeaders {
		c.requireSecret(&problems, c.App.OpenWebUI.InternalBearerSecretRef, "app.openwebui.internal_bearer_secret_ref")
	}

	if c.Cognition.DefaultRoute != "full" && c.Cognition.DefaultRoute != "direct" {
		add("cognition.default_route must be direct or full")
	}
	if c.Cognition.PhaseTimeout.Value() <= 0 {
		add("cognition.phase_timeout must be positive")
	}
	if c.Cognition.PrismConcurrency < 1 || c.Cognition.PrismConcurrency > 5 {
		add("cognition.prism_concurrency must be between 1 and 5")
	}
	if c.Cognition.DegradedMinReports < 1 || c.Cognition.DegradedMinReports > 5 {
		add("cognition.degraded_min_reports must be between 1 and 5")
	}
	if c.Cognition.TokenBudget <= 0 {
		add("cognition.token_budget must be positive")
	}
	if c.Cognition.ToolMaxCalls < 0 {
		add("cognition.tool_max_calls cannot be negative")
	}

	if len(c.Models.Allowlist) == 0 {
		add("models.allowlist cannot be empty")
	}
	for modelID, definition := range c.Models.Allowlist {
		if strings.TrimSpace(modelID) == "" {
			add("models.allowlist contains an empty model ID")
		}
		if definition.ProviderRef == "" {
			add("model %s has no provider_ref", modelID)
		}
		if definition.Protocol != "openai_chat_completions" {
			add("model %s uses unsupported protocol %q", modelID, definition.Protocol)
		}
	}
	for _, role := range requiredModelRoles {
		assignment, ok := c.Models.Roles[role]
		if !ok {
			add("models.roles.%s is required", role)
			continue
		}
		c.validateRole(&problems, role, assignment)
	}
	for role, assignment := range c.Models.Roles {
		if !contains(requiredModelRoles, role) {
			add("models.roles contains unknown runtime role %q", role)
		}
		if assignment.Timeout.Value() <= 0 {
			add("models.roles.%s.timeout must be positive", role)
		}
		if assignment.MaxRetries < 0 {
			add("models.roles.%s.max_retries cannot be negative", role)
		}
		if assignment.MaxOutputTokens <= 0 {
			add("models.roles.%s.max_output_tokens must be positive", role)
		}
	}

	c.validateEndpoint(&problems, "providers.open_code_zen", c.Providers.OpenCodeZen, true)
	c.validateEndpoint(&problems, "providers.langsearch", c.Providers.LangSearch, true)
	c.validateEndpoint(&problems, "providers.qdrant", c.Providers.Qdrant, true)
	if c.Observability.OTLP.Enabled {
		c.validateEndpoint(&problems, "providers.grafana_otlp", c.Providers.GrafanaOTLP, false)
		if c.Observability.OTLP.ProviderRef != "grafana_otlp" {
			add("observability.otlp.provider_ref must be grafana_otlp")
		}
		if c.Observability.OTLP.TraceSample < 0 || c.Observability.OTLP.TraceSample > 1 {
			add("observability.otlp.trace_sample must be between 0 and 1")
		}
	}

	c.requireSecret(&problems, c.Storage.Database.URLRef, "storage.database.url_ref")
	c.requireSecret(&problems, c.Storage.Database.DirectURLRef, "storage.database.direct_url_ref")
	if c.Storage.Database.MaxConnections < 1 {
		add("storage.database.max_connections must be positive")
	}
	if c.Storage.Database.MinConnections < 0 || c.Storage.Database.MinConnections > c.Storage.Database.MaxConnections {
		add("storage.database.min_connections must be between 0 and max_connections")
	}
	if c.Storage.River.Enabled && c.Storage.River.WorkerCount < 1 {
		add("storage.river.worker_count must be positive when River is enabled")
	}
	if c.Storage.ObjectStore.Enabled {
		if c.Storage.ObjectStore.Provider != "r2" {
			add("storage.object_store.provider must be r2 in mini MVP")
		}
		c.requireSecret(&problems, c.Storage.ObjectStore.EndpointRef, "storage.object_store.endpoint_ref")
		c.requireSecret(&problems, c.Storage.ObjectStore.AccessKeyRef, "storage.object_store.access_key_ref")
		c.requireSecret(&problems, c.Storage.ObjectStore.SecretKeyRef, "storage.object_store.secret_key_ref")
		if c.Storage.ObjectStore.Bucket == "" {
			add("storage.object_store.bucket is required when object storage is enabled")
		}
	}

	if c.Retrieval.MemoryCollection == "" || c.Retrieval.DocumentCollection == "" {
		add("retrieval collection names are required")
	}
	if c.Retrieval.TopK <= 0 {
		add("retrieval.top_k must be positive")
	}
	if c.Features.ColBERT != c.Retrieval.ColBERT.Enabled {
		add("features.colbert must match retrieval.colbert.enabled")
	}
	if c.Retrieval.ColBERT.Enabled && c.Retrieval.ColBERT.Model == "" {
		add("retrieval.colbert.model is required when ColBERT is enabled")
	}

	for emotion, value := range c.Emotion.Baseline {
		if value < 0 || value > 1 {
			add("emotion.baseline.%s must be between 0 and 1", emotion)
		}
	}
	if c.Emotion.MaxDelta <= 0 || c.Emotion.MaxDelta > 1 {
		add("emotion.max_delta must be greater than 0 and at most 1")
	}
	if c.Emotion.Relationship.Global && c.Emotion.Relationship.PerUser {
		add("emotion.relationship.global and per_user cannot both be enabled in mini MVP")
	}

	if c.Limits.RequestBytes <= 0 || c.Limits.ManifestBytes <= 0 || c.Limits.ContextTokens <= 0 || c.Limits.ActiveFullPipelines <= 0 {
		add("all core limits must be positive")
	}
	if c.Limits.ManifestBytes > c.Limits.RequestBytes {
		add("limits.manifest_bytes cannot exceed limits.request_bytes")
	}
	if profile == "production" && c.Features.Sandbox {
		add("features.sandbox must remain disabled in production mini MVP")
	}
	if c.Features.DocumentUploads != c.Storage.ObjectStore.Enabled {
		add("features.document_uploads must match storage.object_store.enabled")
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func (c *RuntimeConfig) validateRole(problems *[]string, role string, assignment RoleModelConfig) {
	if assignment.Primary == "" {
		*problems = append(*problems, fmt.Sprintf("models.roles.%s.primary is required", role))
		return
	}
	primary, ok := c.Models.Allowlist[assignment.Primary]
	if !ok {
		*problems = append(*problems, fmt.Sprintf("models.roles.%s references unknown primary model %q", role, assignment.Primary))
	} else if !primary.Enabled {
		*problems = append(*problems, fmt.Sprintf("models.roles.%s primary model %q is disabled", role, assignment.Primary))
	}
	seen := map[string]struct{}{assignment.Primary: {}}
	for _, fallback := range assignment.Fallback {
		if _, duplicate := seen[fallback]; duplicate {
			*problems = append(*problems, fmt.Sprintf("models.roles.%s contains duplicate or cyclic fallback %q", role, fallback))
			continue
		}
		seen[fallback] = struct{}{}
		definition, exists := c.Models.Allowlist[fallback]
		if !exists {
			*problems = append(*problems, fmt.Sprintf("models.roles.%s references unknown fallback model %q", role, fallback))
		} else if !definition.Enabled {
			*problems = append(*problems, fmt.Sprintf("models.roles.%s fallback model %q is disabled", role, fallback))
		}
	}
}

func (c *RuntimeConfig) validateEndpoint(problems *[]string, name string, endpoint EndpointConfig, requireAPIKey bool) {
	if endpoint.Protocol == "" {
		*problems = append(*problems, fmt.Sprintf("%s.protocol is required", name))
	}
	if (endpoint.Endpoint == "") == (endpoint.EndpointRef == "") {
		*problems = append(*problems, fmt.Sprintf("%s must define exactly one of endpoint or endpoint_ref", name))
	} else if endpoint.Endpoint != "" {
		parsed, err := url.ParseRequestURI(endpoint.Endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			*problems = append(*problems, fmt.Sprintf("%s.endpoint is not a valid absolute URL", name))
		}
	} else {
		c.requireSecret(problems, endpoint.EndpointRef, name+".endpoint_ref")
	}
	if requireAPIKey {
		c.requireSecret(problems, endpoint.APIKeyRef, name+".api_key_ref")
	} else if endpoint.HeadersRef != "" {
		c.requireSecret(problems, endpoint.HeadersRef, name+".headers_ref")
	}
}

func (c *RuntimeConfig) requireSecret(problems *[]string, ref, field string) {
	if ref == "" {
		*problems = append(*problems, fmt.Sprintf("%s is required", field))
		return
	}
	value, ok := c.secrets[ref]
	if !ok || value.Empty() {
		*problems = append(*problems, fmt.Sprintf("%s references unresolved secret %q", field, ref))
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
