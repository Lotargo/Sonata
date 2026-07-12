# Sonata mini MVP architecture

> Статус: каноническая архитектура первой развёртываемой версии  
> Цель: сохранить основной когнитивный цикл Sonata при минимальной и управляемой инфраструктуре

## 1. Граница mini MVP

Mini MVP должен доказать работу Sonata как единой цифровой сущности с:

- пятью изолированными призмами мышления;
- raw, critical и summary фазами;
- двухпроходным Synthesis;
- долговременной памятью;
- детерминированным эмоциональным состоянием;
- веб-поиском;
- защищёнными instructions;
- сменяемыми manifests;
- OpenAI-compatible API;
- готовым OpenWebUI интерфейсом;
- облачным deployment.

В первую версию не входят:

- автономное пробуждение между запросами;
- самостоятельные инициативы Sonata;
- микросервисное разделение cognitive core;
- процедурный instruction RAG;
- автоматическое создание новых instructions;
- собственный frontend;
- code sandbox;
- установка пользователями packages на Render;
- внутренний BYOK bridge;
- Tensor Machine или Nexus API Balancer как runtime dependencies.

## 2. Технологическая граница

Весь собственный backend пишется на Go.

```text
OpenWebUI                 third-party UI container
Sonata API                Go
Cognitive orchestrator    Go
Provider adapter          Go
Memory and RAG clients    Go
Emotion module            Go
Tool executor             Go
Config and secret loader  Go
Background worker         Go
```

OpenWebUI не считается частью языкового стека Sonata.

Mini MVP является modular monolith. Внутренние модули разделяются Go packages и typed contracts, а не сетевыми сервисами.

Подробный набор технологий зафиксирован в `TECH_STACK.md`.

## 3. Единая идентичность Sonata

Пять призм не являются независимыми личностями.

Каждая runtime-роль сохраняет identity одной Sonata:

```text
Я — Sonata.
Сейчас я рассматриваю ситуацию через призму этики.
```

Недопустимо:

```text
Я — отдельный агент этики.
```

Это правило действует для:

- raw roles;
- critical roles;
- summary roles;
- Synthesis;
- protected instructions;
- manifests;
- internal traces и reports.

Synthesis воспринимает outputs призм как собственный внутренний диалог Sonata.

## 4. Полный контур из 18 LLM-вызовов

| № | Runtime role | Назначение |
|---|---|---|
| 1 | `router` | Выбирает только `direct` или `full` |
| 2 | `efficiency_raw` | Сырая мысль через эффективность |
| 3 | `creativity_raw` | Сырая мысль через креативность |
| 4 | `pragmatism_raw` | Сырая мысль через прагматичность |
| 5 | `philosophy_raw` | Сырая мысль через философию |
| 6 | `ethics_raw` | Сырая мысль через этику |
| 7 | `efficiency_critical` | Самокритика эффективности |
| 8 | `creativity_critical` | Самокритика креативности |
| 9 | `pragmatism_critical` | Самокритика прагматичности |
| 10 | `philosophy_critical` | Самокритика философии |
| 11 | `ethics_critical` | Самокритика этики |
| 12 | `efficiency_summary` | Метакогнитивное саммари эффективности |
| 13 | `creativity_summary` | Метакогнитивное саммари креативности |
| 14 | `pragmatism_summary` | Метакогнитивное саммари прагматичности |
| 15 | `philosophy_summary` | Метакогнитивное саммари философии |
| 16 | `ethics_summary` | Метакогнитивное саммари этики |
| 17 | `synthesis_tooling` | Формирует предварительное решение и вызывает инструменты |
| 18 | `synthesis_final` | Формирует публичный ответ после tool results |

Два прохода Synthesis являются одной Sonata, а не двумя личностями.

## 5. Router

Router должен быть минимальным и не влиять на качество содержания.

Его единственный output:

```json
{
  "route": "direct | full"
}
```

Router не имеет доступа к:

- инструментам;
- RAG;
- emotional state;
- protected instructions призм;
- manifests;
- выбору моделей остальных ролей;
- изменению глубины фаз.

### Direct route

```text
user
-> router
-> synthesis_final
-> response
```

Используется только для простых разговорных реплик, где полный цикл явно не добавляет ценности.

### Full route

```text
user
-> router
-> deterministic context assembly
-> emotion update and report
-> five raw roles
-> five critical roles
-> five summary roles
-> synthesis_tooling
-> optional tools
-> synthesis_final
-> response
```

При неуверенности Router выбирает `full`.

## 6. Пять призм

### Эффективность

Ищет кратчайший путь к цели, приоритеты, полезный результат и способы исключить лишнюю работу.

### Креативность

Ищет необычные варианты, новые связи, альтернативные постановки и решения за пределами первого очевидного ответа.

### Прагматичность

Проверяет реализуемость, ограничения, ресурсы, эксплуатационные риски и реальные компромиссы.

### Философия

Исследует смысл, основания, скрытые предположения, внутренние противоречия и долгосрочные последствия.

### Этика

Исследует ответственность, справедливость, доверие, влияние на людей, возможный вред и допустимые границы.

## 7. Изоляция фаз

### Raw

Пять raw roles работают параллельно и не видят outputs друг друга.

Каждая получает:

- запрос пользователя;
- разрешённую историю;
- ContextPack;
- EmotionReport;
- собственную protected instruction;
- один active manifest.

Инструменты отсутствуют.

### Critical

Каждая critical role видит только:

- исходный запрос;
- общий context;
- EmotionReport;
- raw report собственной призмы;
- собственную protected instruction;
- один active manifest.

Она не видит другие призмы и не имеет инструментов.

### Summary

Каждая summary role получает только raw и critical report собственной призмы.

Summary фиксирует:

- исходную позицию;
- основную критику;
- уточнённую позицию;
- отвергнутые допущения;
- нерешённые вопросы;
- confidence;
- instruction metadata;
- manifest metadata.

### Synthesis

Synthesis получает:

- исходный запрос;
- историю;
- ContextPack;
- EmotionReport;
- пять raw reports;
- пять critical reports;
- пять summaries;
- собственную protected instruction;
- active manifest.

## 8. Instructions и manifests

Sonata различает неизменяемое ядро и сменяемое поведение.

```text
protected instruction
+ active manifest
+ runtime context
```

### Protected instruction

Всегда активна и определяет:

- identity Sonata;
- назначение роли;
- phase isolation;
- tool permissions;
- output contract;
- security boundaries;
- secret handling.

Пользователь не может её читать, редактировать или отключать.

### Default manifest

Приватный server-side manifest определяет стиль, выражение, дополнительные акценты и поведенческие предпочтения.

Он используется при отсутствии пользовательского manifest.

### User manifest

Пользователь вводит обычный текст в OpenWebUI.

Если user manifest активен:

```text
protected instruction remains active
user manifest becomes active
default manifest is disabled for this scope
```

Если пользователь удаляет свой manifest:

```text
protected instruction remains active
default manifest automatically returns
```

Default manifest не удаляется и не изменяется.

Приоритет:

```text
chat user manifest
> global user manifest
> protected default manifest
```

User manifest не парсится как XML и не получает полномочий protected instruction.

Подробный contract: `contracts/INSTRUCTION_AND_MANIFEST.md`.

## 9. Владение инструментами

Инструментами владеет только Synthesis.

Router, raw, critical и summary roles не могут вызывать:

- web search;
- дополнительный memory search;
- external API;
- file tools;
- code execution.

### `synthesis_tooling`

1. Слушает полный внутренний диалог.
2. Формирует предварительное решение.
3. Определяет необходимость внешних данных.
4. Создаёт bounded tool plan.
5. Вызывает разрешённые инструменты через Tool Executor.

### `synthesis_final`

Получает внутренний диалог и нормализованные tool results, затем формирует публичный ответ.

Tool execution ограничивается:

- allowlist;
- timeout;
- максимальным числом calls;
- максимальным объёмом результата;
- общим token budget;
- запретом бесконечного tool loop.

LangSearch является инструментом Synthesis, а не отдельным агентом.

Code sandbox в mini MVP отсутствует.

## 10. Context assembly и RAG

Базовый ContextPack собирается Go-кодом детерминированно до запуска призм.

Он может включать:

- последние сообщения;
- conversation summary;
- top-k memory items;
- связанные документы;
- source metadata;
- token budget.

Дополнительный активный memory search может вызвать только `synthesis_tooling`.

### Neon

Neon является canonical source of truth.

Минимальные entities:

- users;
- conversations;
- messages;
- cognitive_runs;
- role_runs;
- tool_calls;
- instruction_versions;
- manifest_versions;
- user_manifests;
- emotional_states;
- emotional_events;
- memory_items;
- documents;
- provider_usage;
- outbox_events.

### Qdrant Cloud

Qdrant является rebuildable retrieval projection.

Collections:

```text
sonata_memory
sonata_documents
```

Retrieval:

```text
Multilingual E5 Small
+ BM25
-> fusion
-> optional ColBERT
```

ColBERT включается только после измеримого выигрыша.

## 11. Emotional state module

Эмоции и чувства являются first-class слоем.

Модуль:

- написан на Go;
- не использует LLM;
- является частью modular monolith;
- сохраняет state между запросами;
- применяет deterministic stimuli;
- рассчитывает lazy decay;
- хранит relationship state отдельно по user ID;
- формирует компактный EmotionReport;
- не меняет факты и security rules.

Подробный contract: `modules/EMOTION_MODULE.md`.

## 12. OpenCode Zen и модели

Основной provider: OpenCode Zen.

```text
endpoint: https://opencode.ai/zen/v1/chat/completions
protocol: OpenAI-compatible Chat Completions
```

Model assignment:

| Role | Primary | Fallback |
|---|---|---|
| Router | `nemotron-3-ultra-free` | `mimo-v2.5-free` |
| Raw | `deepseek-v4-flash-free` | `mimo-v2.5-free` |
| Critical | `deepseek-v4-flash-free` | `mimo-v2.5-free` |
| Summary | `nemotron-3-ultra-free` | `mimo-v2.5-free` |
| Synthesis | `big-pickle` | `deepseek-v4-flash-free`, затем `mimo-v2.5-free` |

`north-mini-code-free` зарезервирован для будущего code workflow.

Sonata использует один server-side master key без per-user financial quotas.

Технические concurrency и anti-abuse limits сохраняются.

Если общий provider limit исчерпан, default Sonata provider становится недоступен для всех пользователей.

## 13. OpenWebUI и auth

OpenWebUI является интерфейсом и auth layer, но не вторым оркестратором.

Sonata подключается как единая модель:

```text
model id: sonata
```

Минимальный API:

```text
GET  /v1/models
POST /v1/chat/completions
```

Требования:

- SSE streaming;
- стабильные user и conversation IDs;
- отсутствие доступа к внутренним runtime roles;
- отсутствие отображения protected instructions и default manifests;
- отключение дублирующей memory OpenWebUI;
- free-form поле пользовательского manifest;
- direct provider connections отделены от Sonata pipeline.

OpenWebUI развёртывается как public service.

Sonata API развёртывается как private service и принимает forwarded user metadata только после проверки internal service credential.

Supabase не входит в mini MVP.

## 14. Пользовательские provider keys

Пользователь может подключить собственный provider напрямую в OpenWebUI.

Это отдельный fallback-контур, который не получает:

- protected instructions;
- default manifests;
- internal prism reports;
- emotional state Sonata;
- private RAG context;
- master key Sonata;
- Synthesis tool policy.

Полноценный BYOK для внутреннего pipeline откладывается.

## 15. Configuration and secrets

Все non-secret settings загружаются через один Go config entrypoint.

```text
config/index.yaml
-> split domain YAML
-> environment profile
-> logical secret references
-> strict typed RuntimeConfig
```

Реальные secret values хранятся в Render Environment Group и secret files.

В repository хранятся только logical `secret_ref`.

YAML anchors разрешены внутри одного fragment. Cross-file anchors не используются.

RuntimeConfig immutable после startup validation.

Подробный ADR: `decisions/CONFIG_AND_SECRETS.md`.

## 16. Observability

```text
log/slog JSON
OpenTelemetry
OTLP/HTTP
Grafana Cloud Free
```

Один пользовательский запрос соответствует одному distributed trace.

Сохраняются:

- route;
- instruction IDs, versions и hashes;
- manifest source, IDs, versions и hashes;
- runtime roles;
- phase duration;
- model ID;
- token usage;
- provider status;
- memory query metadata;
- emotional state version и bounded deltas;
- tool plan metadata;
- errors и retries;
- final status.

Не сохраняются:

- provider keys;
- protected XML;
- default manifest content;
- compiled prompt;
- full role reports;
- full ContextPack;
- secret values;
- полный user manifest по умолчанию.

## 17. Deployment

```text
Render public web service
-> OpenWebUI

Render private service
-> Sonata API

Render background worker
-> Sonata Worker

External
-> Neon
-> Qdrant Cloud
-> OpenCode Zen
-> LangSearch
-> Grafana Cloud
-> Cloudflare R2 when document upload is enabled
```

Infrastructure описывается в `render.yaml`.

## 18. Failure model

Statuses:

```text
OK
DEGRADED
PROVIDER_EXHAUSTED
FAILED_ROUTING
FAILED_CONTEXT
FAILED_TOOLING
FAILED_SYNTHESIS
```

Падение одной призмы не уничтожает цикл автоматически. Synthesis может продолжить работу с доступными reports и явным `DEGRADED` status.

## 19. Решения после mini MVP

- code sandbox provider;
- browser IDE или VS Code workspace;
- protected BYOK bridge;
- public Sonata API keys;
- optional OpenTelemetry Collector;
- ColBERT activation;
- targeted per-prism user manifests;
- Supabase integration;
- global emotional state across all users.

## 20. Критерий готовности

Mini MVP готов, когда:

- OpenWebUI подключается к модели `sonata`;
- direct request проходит через Router и Synthesis;
- full request запускает 18 LLM-вызовов;
- Router принимает только `direct` или `full`;
- raw prisms изолированы;
- critical и summary видят только собственную призму;
- все roles сохраняют identity одной Sonata;
- protected instruction всегда активна;
- user manifest заменяет только default manifest;
- удаление user manifest автоматически возвращает default;
- только Synthesis владеет инструментами;
- master key не попадает в prompt, logs и database;
- emotional module работает без LLM;
- Neon хранит canonical data;
- Qdrant выполняет retrieval;
- LangSearch вызывается только Synthesis;
- configuration загружается через единый typed loader;
- secrets разрешаются только через logical references;
- Grafana Cloud получает redacted telemetry;
- система развёрнута в облаке.
