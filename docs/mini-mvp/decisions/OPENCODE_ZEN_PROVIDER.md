# ADR: OpenCode Zen as default model provider

> Статус: accepted for mini MVP  
> Scope: model access, shared credentials, routing, exhaustion and future BYOK

## 1. Decision

OpenCode Zen является default upstream provider Sonata mini MVP.

```text
endpoint: https://opencode.ai/zen/v1/chat/completions
protocol: OpenAI-compatible Chat Completions
```

Sonata использует один server-side master key, общий для всех пользователей.

Индивидуальные финансовые quotas и per-user token budgets отсутствуют.

## 2. Runtime model

```text
OpenWebUI
-> Sonata API
-> Go OpenCodeZenProvider
-> OpenCode Zen chat completions endpoint
-> selected model
```

Для mini MVP нужен один transport. Поддержка Anthropic, Google native и Responses protocols откладывается до появления реальной модели, которой они необходимы.

## 3. Provider interface

```go
type ModelProvider interface {
    Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error)
    Stream(ctx context.Context, request GenerateRequest) (<-chan StreamEvent, error)
    ListModels(ctx context.Context) ([]ModelDescriptor, error)
}
```

Cognitive modules не зависят от upstream request shape.

Provider adapter строится на:

```text
net/http
encoding/json
shared http.Client
shared http.Transport
```

Отдельный SDK provider не используется.

## 4. Model allowlist

Разрешённые модели:

| Display name | Model ID | Runtime use |
|---|---|---|
| Big Pickle | `big-pickle` | Synthesis primary |
| MiMo-V2.5 Free | `mimo-v2.5-free` | общий fallback |
| North Mini Code Free | `north-mini-code-free` | future code workflow only |
| Nemotron 3 Ultra Free | `nemotron-3-ultra-free` | Router и Summary |
| DeepSeek V4 Flash Free | `deepseek-v4-flash-free` | Raw и Critical prisms |

Модель, отсутствующая в allowlist, не может быть активирована автоматически после появления в `/models`.

## 5. Role routing

```yaml
roles:
  router:
    primary: nemotron-3-ultra-free
    fallback:
      - mimo-v2.5-free

  raw:
    primary: deepseek-v4-flash-free
    fallback:
      - mimo-v2.5-free

  critical:
    primary: deepseek-v4-flash-free
    fallback:
      - mimo-v2.5-free

  summary:
    primary: nemotron-3-ultra-free
    fallback:
      - mimo-v2.5-free

  synthesis_tooling:
    primary: big-pickle
    fallback:
      - deepseek-v4-flash-free
      - mimo-v2.5-free

  synthesis_final:
    primary: big-pickle
    fallback:
      - deepseek-v4-flash-free
      - mimo-v2.5-free
```

`north-mini-code-free` не участвует в обычном cognitive pipeline.

Он резервируется для будущих:

- code analysis;
- repository tasks;
- IDE integration;
- sandbox result analysis.

## 6. Master key

Master key загружается только через logical secret reference:

```text
opencode_zen_master_key
-> config/secrets.yaml
-> Render Environment Group
-> OPENCODE_ZEN_API_KEY
```

Master key никогда не должен:

- передаваться в OpenWebUI;
- возвращаться через Sonata API;
- храниться в Neon;
- храниться в Qdrant;
- включаться в protected instructions;
- включаться в manifests;
- включаться в model prompt;
- записываться в logs или traces;
- отображаться в diagnostics;
- копироваться в tool results.

Только Provider Adapter получает resolved `SecretValue`.

## 7. Shared usage policy

```text
one shared master key
+ one shared provider balance or limit
+ one shared allowlist
+ no per-user financial allocation
```

Один пользователь может внести существенный вклад в исчерпание общего limit. Это осознанный компромисс mini MVP.

Sonata сохраняет технические ограничения:

- maximum concurrent requests;
- maximum active full pipelines;
- request timeout;
- maximum context size;
- maximum output size;
- retry budget;
- circuit breaker;
- bounded tool loop;
- базовый anti-abuse control.

Эти ограничения защищают стабильность сервиса и не являются финансовыми quotas.

## 8. Failure and fallback behavior

Fallback выполняется только при model-level failure:

```text
MODEL_UNAVAILABLE
MODEL_TIMEOUT
MODEL_RATE_LIMITED
MODEL_RESPONSE_INVALID
MODEL_PROTOCOL_ERROR
```

Fallback не помогает, если исчерпан общий provider balance или master-key limit.

Provider exhaustion нормализуется в:

```text
PROVIDER_EXHAUSTED
```

Поведение:

1. Прекратить retries, способные увеличить число неуспешных requests.
2. Открыть circuit breaker для provider.
3. Вернуть безопасную понятную ошибку.
4. Зафиксировать cognitive run как failed до завершения generation.
5. Не возвращать raw upstream error и credentials.
6. Не включать скрытый paid fallback.

## 9. User providers in OpenWebUI

Пользователь может настроить собственные provider connections непосредственно в OpenWebUI.

В mini MVP это отдельный direct-provider fallback.

Он не запускает внутренний pipeline Sonata и не получает:

- protected instructions;
- protected default manifests;
- internal prism reports;
- emotional state Sonata;
- private RAG context;
- master key;
- Synthesis tool policy.

UI должен различать:

```text
Sonata model
vs
Direct provider model
```

## 10. Future BYOK bridge

Поздняя версия может позволить user credential питать внутренний pipeline Sonata.

```text
user credential
-> encrypted secret storage or external vault
-> opaque credential reference
-> Sonata Provider Adapter
-> upstream provider
```

Требования:

- encrypted at rest;
- no raw key in application tables;
- no key in prompts, tool calls или traces;
- provider-specific scope;
- explicit user ownership;
- deletion и rotation;
- audit без secret value;
- no cross-user access;
- user-controlled fallback order.

## 11. Model registry

Sonata может синхронизировать model metadata из Zen, но registry не активирует модели самостоятельно.

Metadata:

```yaml
model_id: string
protocol: openai_chat_completions
context_limit: integer
pricing_class: free | paid | unknown
privacy_class: string
status: enabled | disabled | deprecated
approved_roles: list
```

Изменение role routing выполняется только через versioned YAML configuration и deployment review.

## 12. Privacy classes

Каждая модель получает internal privacy class:

```text
standard
retained
training-eligible
restricted
```

Free status модели не означает автоматическую пригодность для private memory или sensitive documents.

Sensitive context передаётся только моделям, разрешённым policy.

## 13. Usage accounting

Sonata сохраняет metadata без credentials:

```yaml
run_id: uuid
user_id: uuid
provider: opencode_zen
model_id: string
runtime_role: string
input_tokens: integer
output_tokens: integer
cached_tokens: integer
estimated_cost: decimal
status: string
created_at: timestamp
```

Это accounting для observability и capacity planning, а не quota enforcement.

## 14. Error normalization

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

Raw provider error bodies не возвращаются ordinary users и проходят redaction до logging.

## 15. Future Sonata proxy API

В будущем Sonata предоставляет собственные API credentials:

```text
IDE or external client
-> Sonata API key
-> Sonata pipeline
-> OpenCode Zen or user BYOK
```

Sonata API key авторизует доступ к Sonata и никогда не раскрывает upstream credential.

## 16. Validation criteria

ADR реализован, когда:

- все LLM calls проходят через `OpenCodeZenProvider`;
- используется один Chat Completions transport;
- role routing совпадает с accepted model table;
- model allowlist enforced;
- master key доступен только через SecretResolver;
- logs и database не содержат key;
- model fallback отличается от provider exhaustion;
- direct OpenWebUI providers визуально отделены от Sonata;
- usage accounting работает без financial quotas;
- tests покрывают secret redaction, model fallback, exhaustion и disabled models.
