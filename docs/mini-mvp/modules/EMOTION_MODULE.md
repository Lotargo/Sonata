# Affective state module boundary

> Статус: каноническая граница модуля для mini MVP  
> Реализация: Go package внутри Sonata modular monolith  
> Текущий код: `internal/emotion` является `v0 core`  
> Математическая и доменная спецификация v1: [`AFFECTIVE_DYNAMICS.md`](./AFFECTIVE_DYNAMICS.md)  
> Донор идей: `Lotargo/Private---Sentio-Engine`

## 1. Назначение

Affective state module поддерживает непрерывное внутреннее состояние Sonata между пользовательскими запросами.

Модуль влияет на:

- тон и эмоциональную выразительность;
- доступность ассоциаций;
- чувствительность к риску;
- доверие и напряжение в отношениях;
- интерпретацию неоднозначных событий;
- итоговый выбор Synthesis.

Модуль не является отдельной личностью и не принимает волевые решения вместо Sonata.

## 2. Разделение документов

Этот документ определяет:

- application boundary;
- ownership и multi-user isolation;
- security restrictions;
- stimulus и report boundary;
- integration с cognitive pipeline;
- storage и degradation rules.

[`AFFECTIVE_DYNAMICS.md`](./AFFECTIVE_DYNAMICS.md) определяет:

- personality и OCEAN;
- physiology и fatigue;
- разные dynamics эмоций;
- excitation и inhibition;
- drives;
- complex states;
- temporal evidence;
- transition equations;
- verification strategy.

При конфликте математической transition-модели приоритет имеет `AFFECTIVE_DYNAMICS.md`.

## 3. Жёсткие границы

Модуль:

- написан на Go;
- не использует LLM как обязательную зависимость;
- не является microservice;
- не выполняет автономные фоновые действия;
- не отправляет сообщения пользователю;
- не вызывает внешние tools;
- не изменяет memory facts;
- не изменяет security policy;
- не имеет доступа к provider keys;
- не зависит от OpenWebUI;
- не принимает готовый state vector от пользователя или модели.

Он является stateful domain module внутри backend.

## 4. Текущий v0 core

В `internal/emotion` уже реализованы:

- typed vector из восьми базовых эмоций;
- typed relationship state;
- per-user `StateKey`;
- validated typed stimuli;
- deterministic lexical extractor;
- bounded v0 transitions;
- opposition/dominance v0;
- lazy exponential decay;
- optimistic compare-and-swap memory store;
- versioned state;
- compact report;
- degraded baseline fallback;
- tests bounds, decay, isolation, replay и concurrent update.

Этот код является полезной основой, но не считается завершённой affective dynamics model.

До HTTP integration он должен быть расширен согласно `AFFECTIVE_DYNAMICS.md`.

## 5. Canonical state ownership

Для mini MVP state изолируется по пользователю:

```text
state_key = sonata_identity_id + user_id
```

Это предотвращает:

- перенос конфликта одного пользователя на другого;
- утечку отношений;
- влияние злоумышленника на ответы всем пользователям;
- смешение private affective history;
- перенос complex states между владельцами.

Global shared affective state Sonata в mini MVP не используется.

## 6. Stimulus boundary

Canonical input является typed event, а не свободным эмоциональным описанием:

```go
type Stimulus struct {
    Kind       StimulusKind
    Source     StimulusSource
    Intensity  Unit
    Confidence Unit
    Valence    SignedUnit
    Arousal    SignedUnit
    Target     StimulusTarget
    CreatedAt  time.Time
    Metadata   SafeMetadata
}
```

Источники сигнала:

- explicit feedback actions UI;
- structured backend events;
- deterministic lexical markers;
- punctuation and formatting markers;
- conversation timing;
- explicit user boundaries;
- tool outcomes;
- response acceptance or rejection;
- cognitive load events.

Один внешний event может быть преобразован в несколько typed stimuli.

LLM classifier может позднее возвращать необязательный `EmotionHint`, но такой hint:

- считается недоверенным;
- проходит schema validation;
- проходит confidence threshold;
- преобразуется только в bounded stimuli;
- никогда не записывается как готовый state.

## 7. Transition boundary

Application вызывает одну детерминированную операцию:

```go
func Transition(
    previous State,
    stimuli []Stimulus,
    now time.Time,
    profile Profile,
) (State, TransitionLog, error)
```

Transition обязан:

- проверить owner и version;
- продвинуть elapsed-time dynamics;
- применить stimuli в стабильном порядке;
- сохранить bounded invariants;
- увеличить version ровно на один;
- вернуть safe audit log без raw text и secrets.

Внутренняя математика описана в `AFFECTIVE_DYNAMICS.md`.

## 8. Report boundary

Affective engine создаёт один canonical report version на cognitive run.

```go
type Report struct {
    StateVersion      int64
    DominantEmotions  []EmotionSignal
    Physiology        PhysiologyReport
    Relationship      RelationshipReport
    ActiveStates      []ComplexStateSignal
    ToneBias          ToneBias
    RiskSensitivity   Unit
    AssociationBiases []AssociationBias
    GeneratedAt       time.Time
}
```

Report:

- компактен;
- не содержит event history;
- не содержит raw user messages;
- не содержит protected instructions;
- не содержит secrets;
- не раскрывает внутренние диагностические коэффициенты.

## 9. Cognitive pipeline integration

Целевой поток:

```text
incoming user event
-> deterministic extraction
-> load affective state
-> Transition
-> persist version
-> build canonical report
-> build role-specific projections
-> cognitive pipeline
-> response/tool outcome events
-> next Transition
```

Router не получает affective report.

Прямой report в mini MVP разрешён:

- Raw prisms;
- Critical phase;
- Synthesis tooling;
- Synthesis final.

Summary phase не получает отдельный report. Она обобщает raw и critical output своей призмы.

Все разрешённые роли одного cognitive run видят одну `StateVersion`.

HTTP integration запрещено отмечать завершённой, пока требования affective dynamics v1 не реализованы и не прошли tests.

## 10. Relationship boundary

Relationship state является частью affective state, но изменяется медленнее basic emotions.

Начальные показатели:

```text
attachment
openness
tension
confidence_in_user
perceived_safety
unresolved_hurt
```

Relationship:

- owner-scoped;
- не даёт permission на autonomous outreach;
- не отменяет user boundaries;
- не меняет security policy;
- не является memory fact store.

## 11. Storage

Canonical storage после этапа 08:

```text
affective_states
affective_events
```

`affective_states` хранит последнюю materialized version, включая:

- emotion vector;
- physiology;
- relationship;
- drives;
- complex states;
- temporal accumulators;
- profile version;
- state version;
- last update timestamp.

`affective_events` хранит:

- typed stimulus kind;
- bounded numeric attributes;
- state version before/after;
- safe audit metadata;
- timestamp.

Raw message text не дублируется в affective tables.

Repository сохраняет optimistic version semantics текущего memory store.

## 12. Security

Модуль не должен:

- принимать произвольный vector напрямую от пользователя;
- доверять XML overlay как источнику state;
- использовать provider output без validation;
- сохранять secret-like metadata;
- позволять одному user ID читать или изменять state другого;
- выводить внутренние events через public API;
- изменять protected instructions;
- использовать active complex state как основание для ослабления safety rules.

User manifest может менять стиль выражения, но не canonical state.

## 13. Graceful degradation

При storage failure или повреждённом state:

```text
load validated baseline profile
-> emit affective_status=DEGRADED
-> continue cognitive pipeline
-> do not overwrite canonical state silently
```

Ошибка affective module не должна полностью останавливать ответ.

Recovery canonical state выполняется отдельной repository operation с audit record.

## 14. Что переносится из Sentio Engine

Переносятся после ревизии:

- stateful emotional core;
- baseline profiles;
- per-emotion dynamics;
- elapsed-time change;
- opposition and dominance;
- OCEAN personality effects;
- relationship persistence;
- drives;
- complex emotional states;
- significant event history;
- bounded report projection.

Не переносятся автоматически:

- Python implementation;
- FastAPI service;
- MongoDB;
- Redis dependency;
- отдельный network API;
- обязательный LLM parser;
- OpenAI proxy mode;
- autonomous background runtime;
- поздние упрощения current `main` Sentio;
- schemas без ревизии.

## 15. Acceptance boundary

Stage 07 готов только когда:

- v1 domain model реализована на Go;
- personality, physiology, drives и complex states участвуют в transition;
- fatigue действует по-разному на разные emotional channels;
- complex states изменяют будущую динамику;
- transition bounded и воспроизводим;
- elapsed time обрабатывается детерминированно;
- state изолирован по user ID;
- модуль не зависит от LLM;
- long-horizon, property, fuzz, replay и race tests проходят;
- один state version используется разрешёнными cognitive roles;
- Router исключён;
- raw content и secrets не попадают в affective event log.
