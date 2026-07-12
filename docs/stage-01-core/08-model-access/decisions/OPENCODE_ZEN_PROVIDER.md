# ADR: OpenCode Zen as default model provider

> Статус: accepted for mini MVP  
> Scope: model access, shared credentials, provider exhaustion and future BYOK

## 1. Decision

OpenCode Zen is the default upstream model provider for Sonata mini MVP.

Sonata uses one server-side master key shared by all users.

There are no individual financial quotas or per-user token budgets in mini MVP.

## 2. Runtime model

```text
OpenWebUI
-> Sonata API
-> Go Provider Adapter
-> OpenCode Zen endpoint
-> selected model
```

The Provider Adapter exposes one internal interface regardless of upstream protocol.

```go
type ModelProvider interface {
    Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error)
    Stream(ctx context.Context, request GenerateRequest) (<-chan StreamEvent, error)
    ListModels(ctx context.Context) ([]ModelDescriptor, error)
}
```

## 3. Supported upstream protocols

OpenCode Zen exposes models through multiple protocol families. The Go adapter must normalize:

- OpenAI Responses API;
- Anthropic Messages API;
- Google native generation API;
- OpenAI-compatible Chat Completions.

Internal cognitive modules must not depend on a provider-specific request shape.

## 4. Master key

The master key is loaded only from deployment secret storage.

It must never be:

- sent to OpenWebUI;
- returned through Sonata API;
- stored in Neon;
- stored in Qdrant;
- included in XML instructions;
- included in model prompts;
- written to logs or traces;
- exposed in admin diagnostics;
- copied into tool results.

Only the Provider Adapter receives the resolved secret at runtime.

## 5. Shared usage policy

All authenticated users use the same default provider pool.

```text
shared balance
shared monthly provider limit
shared model allowlist
no per-user financial allocation
```

One user may consume a significant portion of the shared limit. This is an accepted mini MVP tradeoff.

Sonata still enforces technical protection:

- maximum concurrent requests;
- maximum active full pipelines;
- request timeout;
- maximum context size;
- maximum output size;
- retry budget;
- circuit breaker;
- bounded tool loop.

These controls protect service stability and are not financial quotas.

## 6. Provider exhaustion

The Provider Adapter maps upstream balance and limit failures to:

```text
PROVIDER_EXHAUSTED
```

Expected behavior:

1. Stop automatic retries that would consume additional requests.
2. Open a short circuit breaker for the affected provider or model.
3. Return a clear UI-safe error.
4. Preserve the cognitive run as failed before generation.
5. Do not expose upstream credentials or raw provider response.

No hidden paid fallback is activated automatically.

## 7. User provider keys in OpenWebUI

Users may configure their own provider connections in OpenWebUI.

In mini MVP this is a separate direct-provider fallback. It does not automatically power Sonata's internal 18-call pipeline.

Direct OpenWebUI provider mode does not receive:

- protected Sonata XML;
- internal prism reports;
- Sonata emotional state;
- private Sonata RAG context;
- master key;
- Synthesis tool policy.

The UI must make this distinction clear:

```text
Sonata model
vs
Direct provider model
```

## 8. Future BYOK bridge

A later version may allow a user's provider key to power Sonata itself.

Required design:

```text
user credential
-> encrypted secret storage or vault
-> opaque credential reference
-> Sonata Provider Adapter
-> upstream provider
```

Requirements:

- credentials encrypted at rest;
- no raw key in Neon application tables;
- no key in prompt, tool call or trace;
- provider-specific scope;
- explicit user ownership;
- deletion and rotation;
- audit trail without secret value;
- no cross-user credential access;
- fallback order controlled by user.

## 9. Model registry and allowlist

Sonata may synchronize model metadata from OpenCode Zen, but production selection is restricted by a local allowlist.

```yaml
models:
  router:
    - approved-low-cost-model
  prism:
    - approved-main-model
  summary:
    - approved-low-cost-model
  synthesis:
    - approved-strong-model
```

The allowlist protects against:

- unexpected model appearance;
- deprecated models;
- incompatible protocols;
- models with unsuitable privacy policy;
- accidental use of expensive models;
- behavior changes without review.

Model metadata should include:

- model ID;
- protocol family;
- context limit;
- pricing class;
- privacy class;
- status;
- deprecation date;
- approved runtime roles.

## 10. Privacy classes

Each upstream model receives an internal privacy class:

```text
standard
retained
training-eligible
restricted
```

Private memory and sensitive documents must not be sent to models whose policy allows training or unsuitable retention unless the user explicitly opts in and the model is enabled by policy.

Free model status does not imply suitability for private data.

## 11. Usage accounting

Sonata stores provider usage metadata without credentials:

```yaml
run_id: uuid
user_id: uuid
provider: opencode_zen
model_id: string
input_tokens: integer
output_tokens: integer
cached_tokens: integer
estimated_cost: decimal
status: string
created_at: timestamp
```

This accounting is for observability and capacity planning, not individual quota enforcement.

## 12. Error normalization

Internal errors:

```text
PROVIDER_UNAVAILABLE
PROVIDER_EXHAUSTED
MODEL_DISABLED
MODEL_DEPRECATED
MODEL_PROTOCOL_ERROR
MODEL_TIMEOUT
MODEL_RATE_LIMITED
MODEL_RESPONSE_INVALID
```

Raw provider error bodies are not returned to ordinary users.

## 13. Future Sonata proxy API

Sonata will later expose its own API credentials and act as a provider facade:

```text
IDE or external client
-> Sonata API key
-> Sonata pipeline
-> OpenCode Zen or BYOK
```

A Sonata API key authenticates access to Sonata. It is not an OpenCode Zen key and must never reveal or proxy the upstream credential directly.

## 14. Consequences

Advantages:

- one integration point for many models;
- simple initial operations;
- no individual billing system;
- model changes through configuration;
- compatible future proxy design.

Accepted risks:

- one user can contribute to exhausting the shared balance;
- provider exhaustion affects all default users;
- direct OpenWebUI BYOK does not preserve Sonata pipeline;
- protocol normalization must support several API families;
- privacy differs across individual models.

## 15. Validation criteria

This decision is implemented when:

- all model calls pass through the Go Provider Adapter;
- master key is present only in deployment secret storage;
- no logs or database rows contain the key;
- model allowlist is enforced;
- provider exhaustion returns normalized status;
- direct OpenWebUI providers are visually distinct from Sonata;
- usage accounting works without financial quotas;
- tests cover secret redaction, shared exhaustion and disabled models.
