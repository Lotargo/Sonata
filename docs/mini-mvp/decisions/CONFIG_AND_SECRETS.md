# ADR: unified configuration and secrets

> Статус: accepted for mini MVP  
> Scope: application configuration, secret references, environment profiles and validation

## 1. Decision

Sonata использует одну точку загрузки конфигурации в Go, но не один гигантский YAML-файл.

```text
internal/config
-> load config/index.yaml
-> load domain YAML fragments
-> apply environment profile
-> resolve secret references
-> strict decode into typed Go structs
-> validate
-> expose immutable RuntimeConfig
```

Единая точка управления не означает физическое хранение всех значений в одном файле.

## 2. Technology

```text
go.yaml.in/yaml/v3
+ собственный deterministic loader
+ typed Go config structs
+ explicit Validate() methods
```

Дополнительный универсальный configuration framework для mini MVP не требуется.

Причины:

- порядок загрузки полностью контролируется Sonata;
- нет глобального mutable config object;
- отсутствуют скрытые environment overrides;
- проще обеспечить строгую проверку неизвестных полей;
- проще ограничить merge semantics;
- меньше зависимостей и магии;
- anchors и aliases остаются доступны внутри YAML-файла.

Decoder обязан использовать strict known-field validation.

## 3. Directory structure

```text
config/
├── index.yaml
├── base/
│   ├── app.yaml
│   ├── cognition.yaml
│   ├── models.yaml
│   ├── providers.yaml
│   ├── storage.yaml
│   ├── retrieval.yaml
│   ├── emotion.yaml
│   ├── observability.yaml
│   ├── limits.yaml
│   └── features.yaml
├── environments/
│   ├── local/
│   │   ├── app.yaml
│   │   ├── providers.yaml
│   │   └── observability.yaml
│   ├── staging/
│   └── production/
└── secrets.yaml
```

Environment directories содержат только отличия от `base`, а не полные копии конфигурации.

## 4. Root index

`config/index.yaml` является единственной точкой входа.

```yaml
version: 1

base:
  - base/app.yaml
  - base/cognition.yaml
  - base/models.yaml
  - base/providers.yaml
  - base/storage.yaml
  - base/retrieval.yaml
  - base/emotion.yaml
  - base/observability.yaml
  - base/limits.yaml
  - base/features.yaml

profiles:
  local:
    - environments/local/app.yaml
    - environments/local/providers.yaml
    - environments/local/observability.yaml

  staging:
    - environments/staging/app.yaml
    - environments/staging/providers.yaml
    - environments/staging/observability.yaml

  production:
    - environments/production/app.yaml
    - environments/production/providers.yaml
    - environments/production/observability.yaml

secrets: secrets.yaml
```

Порядок файлов в списке является частью контракта и не должен зависеть от порядка файлов в directory listing.

## 5. Merge precedence

```text
minimal fail-closed code defaults
< base YAML fragments in index order
< selected environment profile fragments in index order
< resolved secret values
```

CLI flags разрешены только для bootstrapping:

```text
--config-root
--profile
--validate-config
--print-config-redacted
```

Произвольное изменение business configuration через environment variables запрещено. Иначе настройки снова окажутся размазаны между YAML, Render Dashboard и runtime shell.

## 6. YAML anchors

Anchors и aliases разрешены внутри одного YAML-файла.

Пример `models.yaml`:

```yaml
shared:
  prism_defaults: &prism_defaults
    provider: opencode_zen
    endpoint: chat_completions
    timeout: 90s
    max_retries: 2

roles:
  efficiency_raw:
    <<: *prism_defaults
    model: deepseek-v4-flash-free

  creativity_raw:
    <<: *prism_defaults
    model: deepseek-v4-flash-free
```

Cross-file anchors запрещены и не поддерживаются.

Для связи между файлами используются стабильные logical IDs:

```yaml
provider_ref: opencode_zen
secret_ref: opencode_zen_master_key
model_policy_ref: prism_default
```

Это явнее и безопаснее, чем скрытая зависимость между YAML documents.

## 7. Domain fragments

### `app.yaml`

- service name;
- environment;
- HTTP address;
- shutdown timeout;
- OpenWebUI trust boundary;
- API compatibility flags.

### `cognition.yaml`

- direct/full route policy;
- phase timeouts;
- concurrency of prisms;
- degraded-mode thresholds;
- token budgets;
- tool-loop limits.

### `models.yaml`

- role-to-model assignments;
- fallback order;
- temperatures;
- max output tokens;
- allowed provider protocol;
- model enable/disable state.

### `providers.yaml`

- OpenCode Zen endpoint;
- LangSearch endpoint;
- Qdrant endpoint metadata;
- Grafana OTLP endpoint metadata;
- logical secret references.

### `storage.yaml`

- Neon pool settings;
- River settings;
- object storage configuration;
- migration policy.

### `retrieval.yaml`

- collections;
- dense and sparse model IDs;
- top-k;
- fusion;
- optional ColBERT feature flag;
- chunking rules.

### `emotion.yaml`

- baseline profile;
- decay rates;
- transition limits;
- relationship-state policy.

### `observability.yaml`

- service names;
- trace sampling;
- metric enablement;
- log level;
- redaction policy;
- OTLP protocol.

### `limits.yaml`

- context limits;
- request size;
- manifest size;
- active full pipelines;
- database pool limits;
- document size.

### `features.yaml`

- sandbox disabled;
- ColBERT enabled/disabled;
- BYOK bridge disabled;
- external public API disabled;
- experimental features.

## 8. Secret registry

`config/secrets.yaml` содержит только logical references, но не secret values.

```yaml
version: 1

secrets:
  opencode_zen_master_key:
    source: env
    key: OPENCODE_ZEN_API_KEY
    required: true

  neon_database_url:
    source: env
    key: DATABASE_URL
    required: true

  langsearch_api_key:
    source: env
    key: LANGSEARCH_API_KEY
    required: true

  qdrant_api_key:
    source: env
    key: QDRANT_API_KEY
    required: true

  grafana_otlp_headers:
    source: file
    path: /etc/secrets/grafana-otlp-headers
    required: true
```

Другие YAML-файлы используют только logical ID:

```yaml
providers:
  opencode_zen:
    api_key_ref: opencode_zen_master_key
```

## 9. Secret sources

Для mini MVP поддерживаются только:

```text
env
file
```

### Environment variables

Подходят для:

- API keys;
- database URLs;
- short tokens;
- internal service secrets.

### Secret files

Подходят для:

- multiline credentials;
- PEM keys;
- OTLP headers;
- certificates;
- structured secret bundles.

Будущий vault добавляется как новый resolver без изменения domain configuration.

## 10. Render policy

В Render используется один Environment Group Sonata для общих secret values и secret files.

Отдельные service-level variables допускаются только для значений, которые действительно отличаются между API и worker.

Нельзя создавать несколько Environment Groups с пересекающимися именами переменных, потому что precedence должен оставаться полностью предсказуемым.

`render.yaml` содержит:

- topology сервисов;
- имена environment groups;
- non-secret deployment wiring;
- placeholders для secret values.

`render.yaml` не содержит:

- API keys;
- database passwords;
- provider credentials;
- business configuration Sonata.

## 11. Go interfaces

```go
type Loader interface {
    Load(ctx context.Context, root string, profile string) (*RuntimeConfig, error)
}

type SecretResolver interface {
    Resolve(ctx context.Context, ref SecretRef) (SecretValue, error)
}
```

`RuntimeConfig` состоит из typed domain structs:

```go
type RuntimeConfig struct {
    App           AppConfig
    Cognition     CognitionConfig
    Models        ModelsConfig
    Providers     ProvidersConfig
    Storage       StorageConfig
    Retrieval     RetrievalConfig
    Emotion       EmotionConfig
    Observability ObservabilityConfig
    Limits        LimitsConfig
    Features      FeaturesConfig
}
```

## 12. Secret value type

Secret values не должны быть обычными strings, которые легко случайно вывести.

```go
type SecretValue struct {
    value string
}

func (s SecretValue) Reveal() string
func (s SecretValue) String() string { return "[REDACTED]" }
```

Дополнительные требования:

- запрет JSON/YAML marshaling raw value;
- redacted `slog.LogValuer`;
- отсутствие `fmt.Printf("%+v", config)` с raw secrets;
- provider adapter получает только нужный ему secret;
- весь RuntimeConfig с секретами не передаётся в domain modules.

## 13. Loading algorithm

```text
read index.yaml
-> validate index version
-> resolve selected profile
-> load each base fragment in declared order
-> load each profile fragment in declared order
-> expand file-local YAML aliases
-> deterministic deep merge
-> load secret registry
-> resolve required secrets
-> strict decode into RuntimeConfig
-> run semantic Validate()
-> freeze config
-> create redacted snapshot
```

Merge rules:

- maps merge recursively;
- scalar from later layer replaces earlier scalar;
- slices replace completely unless a domain explicitly defines another rule;
- type changes are errors;
- unknown fields are errors;
- duplicate logical IDs are errors unless profile override is explicit.

## 14. Immutability

RuntimeConfig загружается один раз при startup и после успешной validation считается immutable.

Mini MVP не поддерживает hot reload application configuration.

Изменение YAML или Render secrets требует restart/redeploy.

Динамические данные не относятся к RuntimeConfig:

- user manifests;
- conversations;
- emotional state;
- model usage;
- memory items;
- feature state, если оно хранится в database как product state.

## 15. Validation

Startup прекращается, если:

- отсутствует required fragment;
- неизвестен profile;
- найден unknown YAML field;
- secret reference не существует;
- required secret не разрешён;
- model role ссылается на неизвестную модель;
- fallback содержит цикл;
- endpoint использует неподдерживаемый protocol;
- лимит имеет недопустимое значение;
- production включает sandbox;
- protected instruction или manifest registry не согласуется с model role registry.

CLI:

```text
sonata config validate --profile production
sonata config print --profile production --redacted
```

## 16. Redacted diagnostics

Admin diagnostics могут вернуть только redacted snapshot:

```yaml
providers:
  opencode_zen:
    endpoint: https://opencode.ai/zen/v1/chat/completions
    api_key: "[REDACTED]"
    api_key_source: env

models:
  router: nemotron-3-ultra-free
  prism: deepseek-v4-flash-free
  summary: nemotron-3-ultra-free
  synthesis: big-pickle
```

Snapshot содержит config version, profile, file hashes и secret source names, но не secret values.

## 17. Repository policy

В repository хранятся:

- YAML configuration;
- secret registry with references;
- `.env.example` только с именами переменных;
- config schema tests;
- redacted examples.

В repository не хранятся:

- `.env`;
- Render secret exports;
- raw API keys;
- database credentials;
- Grafana credentials;
- user provider keys.

## 18. Критерий готовности

Решение реализовано, когда:

- существует один `internal/config` entrypoint;
- `config/index.yaml` определяет полный порядок загрузки;
- domain YAML разделены по ответственности;
- anchors работают внутри fragment;
- cross-file anchors отсутствуют;
- секреты представлены logical references;
- Render хранит реальные secret values;
- unknown fields и type changes завершают startup ошибкой;
- RuntimeConfig immutable;
- redacted snapshot не раскрывает secrets;
- tests покрывают merge precedence, missing secrets, anchors, invalid profiles и redaction.
