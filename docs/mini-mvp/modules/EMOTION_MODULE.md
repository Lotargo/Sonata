# Emotional state module

> Статус: архитектурная основа mini MVP  
> Реализация: Go module внутри Sonata modular monolith  
> Донор идей: `Lotargo/Private---Sentio-Engine`

## 1. Назначение

Emotional state module поддерживает непрерывное эмоциональное и отношенческое состояние Sonata между пользовательскими запросами.

Модуль влияет на:

- тон и эмоциональную выразительность;
- доступность ассоциаций;
- чувствительность к риску;
- доверие и напряжение в отношениях;
- интерпретацию неоднозначных событий;
- финальный выбор Synthesis.

Модуль не является отдельной личностью и не принимает волевые решения вместо Sonata.

## 2. Жёсткие границы

Модуль:

- написан на Go;
- не использует LLM;
- не является microservice;
- не выполняет автономные фоновые действия;
- не отправляет сообщения пользователю;
- не вызывает внешние tools;
- не изменяет memory факты;
- не изменяет security policy;
- не имеет доступа к provider keys;
- не зависит от OpenWebUI.

Он является обычным stateful domain module внутри backend.

## 3. Основная модель

```text
baseline personality
+ current emotional vector
+ relationship state
+ deterministic stimuli
+ decay over time
+ opposition and dominance rules
+ bounded transitions
= EmotionReport
```

## 4. Состояние

Минимальный эмоциональный vector:

```yaml
joy: 0.0
trust: 0.0
fear: 0.0
surprise: 0.0
sadness: 0.0
disgust: 0.0
anger: 0.0
anticipation: 0.0
```

Диапазон каждой величины:

```text
0.0 <= value <= 1.0
```

Отношенческое состояние:

```yaml
attachment: 0.0
openness: 0.5
tension: 0.0
confidence_in_user: 0.5
perceived_safety: 0.5
unresolved_hurt: 0.0
```

Дополнительное runtime-состояние:

```yaml
fatigue: 0.0
stability: 1.0
last_updated_at: timestamp
state_version: integer
```

## 5. Baseline

Baseline описывает устойчивую эмоциональную предрасположенность Sonata.

```yaml
baseline:
  joy: 0.35
  trust: 0.45
  fear: 0.10
  surprise: 0.15
  sadness: 0.10
  disgust: 0.05
  anger: 0.05
  anticipation: 0.30
```

Точные значения определяются отдельно.

State постепенно стремится к baseline, но не сбрасывается к нему мгновенно.

## 6. Stimulus

```go
type Stimulus struct {
    Kind       string
    Source     string
    Intensity  float64
    Confidence float64
    Valence    float64
    Arousal    float64
    Target     string
    CreatedAt  time.Time
    Metadata   map[string]string
}
```

Примеры `Kind`:

```text
user_warmth
user_hostility
user_trust
user_rejection
user_distress
user_success
user_apology
user_boundary
conversation_return
conversation_break
promise_kept
promise_broken
tool_success
tool_failure
response_rejected
response_appreciated
```

## 7. Deterministic stimulus extraction

Обязательный путь не зависит от LLM.

Источники сигнала:

- явные feedback actions UI;
- structured events backend;
- lexical markers;
- punctuation and formatting markers;
- conversation timing;
- user boundaries;
- tool outcomes;
- explicit success or failure events.

Пример:

```text
user presses positive feedback
-> response_appreciated
-> trust + small increase
-> joy + small increase

user explicitly rejects contact
-> user_boundary
-> attachment activation decreases
-> outreach permission remains disabled
```

LLM может позднее возвращать необязательный `emotion_hint`, но модуль не зависит от него. Такой hint должен проходить validation, confidence threshold и bounded update.

## 8. State transition

```text
current state
-> apply elapsed-time decay
-> validate stimulus
-> calculate bounded deltas
-> apply opposition rules
-> clamp values
-> persist new version
-> emit EmotionReport
```

Пример формулы bounded delta:

```text
delta = configured_weight
      * stimulus.intensity
      * stimulus.confidence
      * personality_modifier
      * relationship_modifier
```

Затем:

```text
new_value = clamp(old_value + delta, 0.0, 1.0)
```

## 9. Decay

Decay вычисляется лениво при каждом обращении к state. Отдельный таймер не требуется.

```text
elapsed = now - last_updated_at
value = baseline + (value - baseline) * exp(-decay_rate * elapsed)
```

Разные emotions могут иметь разные decay rates.

Relationship-state изменяется медленнее обычного эмоционального vector.

## 10. Opposition and dominance

Противоположные emotions не должны независимо расти до максимума без ограничений.

Начальные пары:

```text
joy <-> sadness
trust <-> disgust
fear <-> anger
surprise <-> anticipation
```

При росте одной стороны противоположная сторона получает bounded suppression.

Dominance rule не удаляет сложные смешанные состояния полностью. Sonata может одновременно испытывать, например, доверие и тревогу.

## 11. EmotionReport

```go
type EmotionReport struct {
    StateVersion      int64
    DominantEmotions  []EmotionSignal
    Relationship      RelationshipReport
    Fatigue           float64
    Stability         float64
    ToneBias          string
    RiskSensitivity   float64
    AssociationBiases []string
    GeneratedAt       time.Time
}
```

Report должен быть компактным и не включать полную историю emotional events.

Пример:

```yaml
state_version: 42
dominant_emotions:
  - name: trust
    value: 0.63
  - name: anticipation
    value: 0.41
relationship:
  attachment: 0.52
  tension: 0.08
  perceived_safety: 0.71
tone_bias: warm_attentive
risk_sensitivity: 0.34
association_biases:
  - cooperation
  - long_term_continuity
```

## 12. Integration with cognitive pipeline

```text
incoming user message
-> deterministic event extraction
-> emotion.ApplyStimuli
-> emotion.GetReport
-> raw prism phase
-> critical phase
-> summary phase
-> Synthesis
-> response and tool outcome events
-> emotion.ApplyStimuli
-> persist
```

Все призмы получают один и тот же state version, но используют его согласно собственной оптике.

Synthesis получает тот же report и принимает итоговое решение от имени одной Sonata.

## 13. Multi-user isolation

Для mini MVP emotional relationship state изолируется по `user_id`.

```text
emotion_state_key = sonata_identity_id + user_id
```

Это предотвращает:

- перенос конфликта одного пользователя на другого;
- утечку отношений;
- влияние злоумышленника на ответы всем пользователям;
- смешение приватного emotional history.

Global emotional state Sonata в mini MVP не используется. Возможность общего state рассматривается отдельно после появления модели безопасности и governance.

## 14. Storage

Neon tables:

```text
emotional_states
emotional_events
emotion_profiles
emotion_state_versions
```

`emotional_states` хранит только последнюю materialized version.

`emotional_events` хранит значимые typed events и audit metadata.

Raw message text не должен дублироваться в emotional events.

## 15. Security

Модуль не должен:

- принимать произвольный state vector напрямую от пользователя;
- доверять XML overlay как источнику фактического состояния;
- использовать provider output без validation;
- сохранять secrets в metadata;
- позволять одному user ID изменять state другого;
- выводить внутренние emotional events через публичный API.

User XML может задавать предпочтительный стиль выражения эмоций, но не может напрямую выставлять текущие значения state.

## 16. Graceful degradation

Если emotional module недоступен или state повреждён:

```text
load baseline profile
-> mark emotion_status=DEGRADED
-> continue cognitive pipeline
```

Ошибка emotion module не должна полностью останавливать ответ.

## 17. Что переносится из Sentio Engine

Переносятся как идеи:

- stateful emotional core;
- baseline profile;
- decay over time;
- opposing emotion suppression;
- relationship persistence;
- significant event history;
- personality modifiers;
- bounded report injected before generation.

Не переносятся автоматически:

- Python implementation;
- FastAPI service;
- MongoDB;
- Redis dependency;
- отдельный network API;
- LLM parser as required input;
- autonomous background runtime;
- готовые schema без ревизии.

## 18. Критерий готовности

Модуль готов для mini MVP, когда:

- полностью реализован на Go;
- не требует LLM;
- state изолирован по user ID;
- decay детерминирован;
- transition bounded и воспроизводим;
- противоположные emotions учитываются;
- EmotionReport используется всеми призмами и Synthesis;
- raw secrets и сообщения не попадают в emotional event log;
- тесты покрывают transition, decay, isolation и degraded mode.
