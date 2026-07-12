# Sonata mini MVP architecture

> Статус: зафиксированная основа для дальнейших уточнений  
> Этап: `stage-01-core`  
> Цель: получить развёртываемую версию Sonata без упрощения основного когнитивного цикла

## 1. Граница mini MVP

Mini MVP должен выйти за пределы PoC и доказать работу Sonata как единой цифровой сущности с многопризменным мышлением, долговременной памятью, эмоциональным состоянием, внешними инструментами и наблюдаемым когнитивным циклом.

Упрощается инфраструктура, но не основной pipeline.

В первую версию входят:

- OpenWebUI как готовый интерфейс;
- OpenAI-compatible backend Sonata;
- backend и все собственные runtime-модули на Go;
- OpenCode Zen как основной модельный provider;
- один общий master key OpenCode Zen;
- Router для выбора короткого или полного маршрута;
- пять изолированных призм;
- фазы сырого ответа, самокритики и саммари;
- двухпроходный Synthesis, единственный владелец инструментов;
- XML-инструкции и пользовательские XML overrides;
- приватные server-side default instructions;
- детерминированный эмоциональный модуль без LLM;
- Neon как каноническая PostgreSQL;
- Qdrant Cloud для RAG;
- LangSearch для веб-поиска;
- облачное развёртывание.

В первую версию не входят:

- автономное пробуждение между запросами;
- самостоятельные инициативы Sonata;
- микросервисное разделение когнитивного ядра;
- процедурный instruction RAG;
- автоматическое создание новых инструкций;
- собственный пользовательский интерфейс;
- установка пользователями пакетов в runtime Render;
- обязательная интеграция Tensor Machine или Nexus API Balancer.

## 2. Технологическая граница

Весь собственный backend Sonata пишется на Go.

```text
OpenWebUI                 third-party UI container
Sonata backend            Go
Cognitive orchestrator    Go
Provider adapters         Go
Memory and RAG clients    Go
Emotion module            Go
Tool executor             Go
API and streaming         Go
```

OpenWebUI не считается частью языкового стека Sonata: это готовая внешняя оболочка, которая развёртывается отдельным контейнером.

На этапе mini MVP используется модульный монолит. Сетевое разделение внутренних модулей не допускается без доказанной эксплуатационной причины.

Основной принцип:

```text
explicit cognitive state machine
+ declarative runtime role registry
+ private versioned XML instructions
+ parallel prism execution
+ deterministic emotional state
+ bounded tool execution
+ observable runs
```

## 3. Единая идентичность Sonata

Пять призм не являются независимыми личностями и не представляются как отдельные агенты.

Каждая runtime-роль должна сохранять идентичность Sonata:

```text
Я — Sonata.
Сейчас я рассматриваю ситуацию через призму этики.
```

Недопустимая формулировка:

```text
Я — агент этики.
```

Это правило действует для:

- raw phase;
- critical phase;
- summary phase;
- Synthesis;
- внутренних trace и prompt templates.

Synthesis воспринимает отчёты как собственные внутренние перспективы Sonata, а не как сообщения коллектива внешних экспертов.

Отдельные имена, персонажи и маркеры под-личностей в mini MVP не используются.

## 4. Полный контур из 18 LLM-вызовов

Полный маршрут сохраняет 18 логических LLM-вызовов без отдельного поискового агента.

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
| 17 | `synthesis_tooling` | Сбор внутреннего решения и вызов инструментов |
| 18 | `synthesis_final` | Финальный ответ той же Sonata после результатов инструментов |

`synthesis_tooling` и `synthesis_final` являются двумя проходами одного Synthesis, а не двумя личностями.

## 5. Router

Router должен быть минимальным и не участвовать в качестве ответа.

Он не имеет доступа к:

- инструментам;
- RAG;
- emotional state;
- XML-инструкциям призм;
- модельному выбору призм;
- изменению prompt;
- выбору глубины отдельных фаз.

Единственное решение Router:

```json
{
  "route": "direct | full"
}
```

### Direct route

Используется только для простых разговорных реплик:

```text
user
-> router
-> synthesis_final
-> response
```

Примеры:

- приветствие;
- прощание;
- короткая бытовая реакция;
- ответ, где полноценный внутренний цикл явно не добавляет ценности.

### Full route

Используется для советов, разбора ситуаций, технического анализа, неоднозначных вопросов и запросов средней или высокой сложности.

```text
user
-> router
-> deterministic context assembly
-> emotion update and report
-> 5 raw roles
-> 5 critical roles
-> 5 summary roles
-> synthesis_tooling
-> optional tools
-> synthesis_final
-> response
```

Если Router не уверен, выбирается `full`.

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

### Фаза 1: raw

Пять призм работают параллельно и не видят ответы друг друга.

Общие входы:

- запрос пользователя;
- разрешённая история;
- детерминированно собранный `ContextPack`;
- `EmotionReport`;
- собственная защищённая XML-инструкция;
- пользовательский XML overlay, если он задан.

Призмы не имеют инструментов.

### Фаза 2: critical

Каждая критическая роль видит только:

- исходный запрос;
- общий context;
- emotional report;
- raw report собственной призмы.

Она не видит другие призмы и не имеет инструментов.

### Фаза 3: summary

Каждая summary-роль видит только raw и critical report собственной призмы.

Саммари фиксирует:

- исходную позицию;
- основную критику;
- уточнённую позицию;
- отвергнутые допущения;
- нерешённые вопросы;
- уверенность;
- ID и версию инструкции.

### Synthesis

Synthesis получает:

- исходный запрос;
- историю;
- ContextPack;
- EmotionReport;
- пять raw reports;
- пять critical reports;
- пять summaries.

Он воспринимает их как собственный внутренний диалог Sonata.

## 8. Владение инструментами

Инструментами владеет только Synthesis.

Ни Router, ни призмы, ни критики, ни суммаризаторы не могут вызывать:

- web search;
- дополнительный memory search;
- code execution;
- file tools;
- внешние API.

### Проход `synthesis_tooling`

Этот проход:

1. Слушает полный внутренний диалог.
2. Формирует предварительное решение.
3. Определяет, требуются ли внешние данные или выполнение кода.
4. Создаёт ограниченный структурированный tool plan.
5. Вызывает разрешённые инструменты через Tool Executor.

### Проход `synthesis_final`

Этот проход получает внутренний диалог и нормализованные результаты инструментов, после чего формирует публичный ответ.

Вызовы инструментов ограничиваются:

- allowlist;
- timeout;
- максимальным числом вызовов;
- максимальным объёмом результата;
- общим token budget;
- запретом рекурсивного бесконечного tool loop.

LangSearch является инструментом Synthesis, а не отдельным агентом.

## 9. Context assembly и RAG

Базовый context собирается backend-кодом детерминированно до запуска призм. Это инфраструктурная операция, а не агент и не tool choice.

ContextPack может включать:

- последние сообщения;
- summary диалога;
- top-k релевантных элементов памяти;
- связанные документы;
- metadata источников;
- token budget.

Активный дополнительный поиск по памяти может вызвать только `synthesis_tooling`.

### Neon

Neon является каноническим источником истины.

Минимальные сущности:

- users;
- conversations;
- messages;
- cognitive_runs;
- role_runs;
- tool_calls;
- instruction_versions;
- user_instruction_overlays;
- emotional_states;
- emotional_events;
- memory_items;
- documents;
- provider_usage;
- outbox_events.

### Qdrant Cloud

Qdrant является пересобираемой retrieval-проекцией.

Начальные коллекции:

```text
sonata_memory
sonata_documents
```

Предварительные модели:

| Назначение | Модель |
|---|---|
| Dense retrieval | `Intfloat Multilingual E5 Small` |
| Dense fallback | `All MiniLM L6 v2` |
| Sparse retrieval | `BM25` |
| Late interaction | `Answer.AI Colbert Small V1` |

Начальная схема:

```text
dense + BM25
-> fusion
-> optional ColBERT
-> top context
```

ColBERT включается feature flag и сохраняется только при измеримом выигрыше.

## 10. OpenCode Zen provider

Основным provider является OpenCode Zen.

Sonata использует один server-side master key с доступом к разрешённым моделям Zen.

Master key:

- хранится только в secret storage окружения;
- никогда не передаётся в OpenWebUI;
- никогда не включается в prompt;
- никогда не записывается в logs, traces или database;
- не возвращается через API;
- не доступен пользовательским XML overlays.

Go Provider Adapter нормализует разные API-протоколы моделей Zen за единым внутренним контрактом:

```text
OpenAI Responses
Anthropic Messages
Google native models
OpenAI-compatible Chat Completions
```

Model Registry периодически получает доступный список моделей, но production использует отдельный allowlist.

### Общий лимит

Индивидуальных token или credit quotas для пользователей нет.

```text
one shared master key
+ one shared provider balance or limit
+ no per-user budget allocation
```

При этом сохраняются технические ограничения безопасности:

- concurrency limit;
- request timeout;
- максимальный размер контекста;
- защита от бесконечных повторов;
- базовый anti-abuse rate control.

Они не являются индивидуальными финансовыми лимитами.

Если общий key или provider limit исчерпан, default Sonata provider временно недоступен для всех пользователей.

## 11. Пользовательские provider keys

В OpenWebUI пользователь может настроить собственные подключения к нужным providers.

Для mini MVP это отдельный fallback-контур OpenWebUI. Такой прямой provider route не должен получать:

- master key Sonata;
- приватные default instructions;
- внутренние reports призм;
- emotional state;
- закрытый RAG context.

Полноценный режим, в котором пользовательский key питает именно внутренний pipeline Sonata, откладывается до появления защищённого BYOK bridge.

Будущий BYOK bridge должен:

- принимать credential через защищённый endpoint;
- хранить его зашифрованно или использовать внешний secret vault;
- передавать в pipeline только opaque credential reference;
- никогда не помещать key в prompt;
- поддерживать provider-specific adapters;
- позволять удалить credential;
- вести audit без раскрытия секрета.

## 12. XML-инструкции

Все инструкции runtime-ролей хранятся в XML.

Существующие JSON prompts из `.artifacts/prompts_sonata` являются источником миграции, но runtime-формат mini MVP — XML.

Слои инструкции:

```text
protected identity core
+ protected role defaults
+ protected output contract
+ user XML overlay
+ runtime context
```

### Default instructions

Default XML:

- хранится только server-side;
- не отдаётся через UI и API;
- не отображается в пользовательских traces;
- загружается по ID и версии;
- в logs представляется только hash и version;
- не может быть прочитан пользовательским overlay.

### User XML overlay

Пользователь может написать собственные инструкции через UI.

Overlay может изменять разрешённые параметры поведения своей Sonata, но не может:

- запросить server-side XML;
- раскрыть provider keys;
- изменить security policy;
- выдать инструменты призмам;
- отменить изоляцию фаз;
- изменить идентичность Sonata на набор независимых агентов;
- получить internal traces другого пользователя.

Protected XML и user XML хранятся раздельно и компилируются только внутри backend.

### Защита от раскрытия

Keys никогда не должны попадать в model context, поэтому их раскрытие через prompt невозможно архитектурно.

Для protected instructions применяется best-effort защита:

- запрет выдачи system и developer instructions;
- отсутствие raw prompt в API и logs;
- output redaction для точных или длинных совпадений с protected XML;
- отдельные hashes защищённых fragments;
- минимизация текста protected prompt.

Защищённые инструкции нельзя считать криптографически невыводимым секретом после передачи LLM, поэтому критические секреты в них не хранятся.

## 13. Emotional state module

Эмоции и чувства являются обязательным first-class слоем mini MVP.

За основу берутся идеи `Private---Sentio-Engine`, но новая реализация:

- пишется на Go;
- является внутренним автономным модулем modular monolith;
- не является отдельным microservice;
- не требует LLM;
- не выполняет самостоятельные фоновые действия;
- не создаёт отдельную личность;
- сохраняет состояние между запросами.

Основные функции:

- baseline emotional profile;
- deterministic stimulus processing;
- gradual decay;
- suppression конфликтующих emotions;
- relationship state;
- significant emotional events;
- bounded state transitions;
- создание компактного `EmotionReport`.

Поток:

```text
user event
-> deterministic stimulus extractor
-> emotion state transition
-> EmotionReport
-> five prisms and Synthesis
-> response and tool outcome events
-> final bounded state update
```

Emotional module влияет на доступность ассоциаций, тон, чувствительность к риску и способ Синтеза, но не переписывает факты и не отменяет security rules.

Для публичного multi-user режима состояние отношений хранится отдельно по user ID. Решение о наличии общего глобального emotional state Sonata принимается позднее.

Подробный контракт описывается отдельно в `EMOTION_MODULE.md`.

## 14. OpenWebUI

OpenWebUI является интерфейсом, а не вторым оркестратором.

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

- streaming;
- стабильные user и conversation IDs;
- отсутствие прямого доступа к внутренним runtime-ролям;
- отсутствие отображения protected XML;
- отсутствие отображения master key;
- отключение дублирующей памяти OpenWebUI;
- пользовательский XML editor хранит только user overlay;
- direct provider connections отделены от Sonata pipeline.

## 15. Code workspace и sandbox

Render backend не используется как пользовательская машина разработки.

Пользователю запрещено:

- устанавливать packages в процесс Sonata;
- менять системные libraries;
- выполнять произвольные shell-команды в основном контейнере;
- сохранять исполняемый код между пользователями;
- получать доступ к secrets или внутренней сети.

Будущий code workspace должен быть отдельным изолированным runtime с:

- immutable image;
- заранее установленными toolchains и libraries;
- временной filesystem;
- CPU, RAM и disk limits;
- timeout;
- disabled network по умолчанию;
- allowlist network при необходимости;
- уничтожением workspace после завершения;
- отдельным audit log.

VS Code или browser IDE может быть интерфейсом к такому workspace, но не заменяет саму sandbox.

SourceCraft CLI остаётся кандидатом для development workflow, а не подтверждённым runtime пользовательского кода.

## 16. Public API и proxy mode

В будущем Sonata предоставляет внешний API и может подключаться как модель в IDE, coding agent или другой среде.

```text
client environment
-> Sonata OpenAI-compatible API
-> cognitive pipeline
-> OpenCode Zen or user BYOK provider
-> response
```

Sonata выступает как:

- model provider facade;
- cognitive orchestrator;
- controlled proxy layer;
- memory and emotion layer;
- tool-owning Synthesis runtime.

Будущий API должен использовать собственные Sonata API keys, scopes, audit и технические rate limits, не раскрывая upstream provider credentials.

## 17. Развёртывание

Предварительная схема:

```text
OpenWebUI container
+ Go Sonata backend
-> Render

Canonical PostgreSQL
-> Neon

Retrieval and hosted embedding models
-> Qdrant Cloud

Default model provider
-> OpenCode Zen

Web search
-> LangSearch
```

Supabase пока не является обязательной частью runtime. Его роль будет пересмотрена отдельно после выбора окончательной схемы Auth и admin control plane.

## 18. Наблюдаемость

Для каждого запроса сохраняются:

- route;
- XML instruction IDs, versions и hashes;
- активированные runtime-роли;
- время каждой фазы;
- model ID и provider protocol;
- token usage;
- общий provider status;
- memory queries;
- emotional state version и bounded deltas;
- tool plan и tool calls;
- errors и retries;
- итоговый status.

Не сохраняются:

- master key;
- пользовательские provider secrets в открытом виде;
- compiled raw protected prompt;
- полный protected XML;
- secrets из environment.

Статусы:

```text
OK
DEGRADED
PROVIDER_EXHAUSTED
FAILED_ROUTING
FAILED_CONTEXT
FAILED_TOOLING
FAILED_SYNTHESIS
```

Падение одной призмы не уничтожает цикл автоматически. Synthesis может продолжить работу с доступными отчётами и явным degraded status.

## 19. Нерешённые решения

1. Конкретный Go HTTP router и набор libraries.
2. Модель OpenCode Zen для каждой runtime-роли.
3. Model allowlist и privacy policy для free endpoints.
4. Точная схема пользовательского XML editor в OpenWebUI.
5. Защищённый BYOK bridge для внутреннего pipeline Sonata.
6. Нужен ли ColBERT в первой версии.
7. Безопасный provider sandbox или self-hosted runtime.
8. JSON Schema внутренних reports и tool plan.
9. Правила сохранения новых memory items.
10. Глобальный или только relationship emotional state.
11. Окончательная роль Supabase.

## 20. Критерий готовности

Mini MVP готов, когда:

- OpenWebUI подключается к модели `sonata`;
- простой запрос проходит через Router и Synthesis;
- сложный запрос запускает полный контур из 18 LLM-вызовов;
- Router принимает только решение direct или full;
- пять raw-призм изолированы;
- critical и summary работают только со своей призмой;
- все runtime-роли сохраняют идентичность одной Sonata;
- только Synthesis владеет инструментами;
- XML default instructions недоступны через UI и API;
- user XML overlay работает без доступа к protected layers;
- master key OpenCode Zen не попадает в prompt, logs и database;
- emotional module работает без LLM;
- Neon хранит canonical data;
- Qdrant Cloud выполняет retrieval;
- LangSearch вызывается только Synthesis;
- provider exhaustion корректно возвращает отдельный status;
- система развёрнута в облаке.
