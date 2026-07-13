# Affective dynamics engine

> Статус: accepted design contract для mini MVP stage 07  
> Реализация: Go package внутри Sonata modular monolith  
> Приоритет: этот документ уточняет и заменяет упрощённую transition-модель из `EMOTION_MODULE.md`  
> Источник: архитектурная археология `Lotargo/Private---Sentio-Engine` и подтверждённый автором исходный замысел

## 1. Решение

Sonata должна использовать не простой sentiment state и не набор независимых эмоциональных шкал, а детерминированную affective dynamics system.

Система моделирует:

- устойчивые черты личности;
- физиологическое состояние;
- восемь базовых эмоций;
- отношения с конкретным пользователем;
- внутренние drives;
- краткосрочное возбуждение и торможение;
- долговременные составные состояния;
- обратное влияние составных состояний на будущие реакции;
- непрерывное изменение между пользовательскими запросами.

LLM может получать только компактную проекцию результата. LLM не вычисляет canonical emotional state и не может напрямую задавать его значения.

## 2. Что восстановлено из Sentio Engine

В истории Sentio подтверждены следующие механики:

- индивидуальный baseline каждой эмоции;
- отдельная скорость затухания каждой эмоции;
- синхронизация с реально прошедшим временем;
- пары противоположных эмоций по модели Плутчика;
- подавление слабой эмоции более сильной противоположностью;
- OCEAN-профиль и модификация силы стимула чертами личности;
- история эмоциональных значений;
- обнаружение сложных состояний по устойчивому выполнению условий на временном окне;
- drives как запланированный слой мотивации.

Ключевые исторические точки Sentio:

- `96d0df114677b720c7511fd35c40e22e33a16e94` — time-based decay;
- `a6cc0203839ca959ba4900092c2f8b4ecc316275` — opposition and dominance;
- `cded11f57aaaf9c7e63b6b133a807f7eee4ed83c` — complex emotional states;
- `5f57c0e891a83dea9a3c6dab7171e4ee379b071a` — OCEAN personality modifiers.

Текущий `main` Sentio не считается полной спецификацией. Поздняя LLM-интеграция, переход к proxy architecture и последующие упрощения изменили или удалили часть первоначального замысла.

## 3. Подтверждённый исходный замысел

Помимо восстановленного кода, для Sonata принимаются следующие авторские требования:

- эмоции имеют разную динамику возбуждения и восстановления;
- усталость может притуплять чувства, но её эффект зависит от конкретной эмоции;
- некоторые физиологические состояния могут уменьшать положительную реактивность и одновременно ослаблять контроль раздражения;
- продолжительная грусть может перейти в депрессивное состояние;
- депрессивное состояние не является только меткой: оно подавляет прирост радости, замедляет восстановление грусти и меняет будущую восприимчивость;
- составные состояния образуют обратные связи и могут поддерживать сами себя;
- одинаковый внешний стимул должен давать разные реакции при разных personality, physiology, relationship и complex state;
- ядро должно оставаться полностью работоспособным без LLM.

## 4. Неподвижные границы

Affective engine:

- реализуется на Go;
- является внутренним domain package, а не microservice;
- не вызывает LLM и tools;
- не имеет provider credentials;
- не меняет protected instructions;
- не меняет security policy;
- не записывает memory facts;
- не принимает произвольный state vector от пользователя;
- не сохраняет raw message text в emotional event log;
- не имеет глобального shared state между пользователями в mini MVP;
- даёт воспроизводимый результат для одинаковой последовательности versioned events.

## 5. Слои состояния

Canonical state состоит из семи слоёв:

```text
Personality
+ Physiology
+ Basic emotions
+ Relationship
+ Drives
+ Complex states
+ Temporal evidence
= AffectiveState
```

### 5.1. Personality

Personality описывает устойчивую восприимчивость, а не текущее настроение.

Минимальный профиль:

```go
type Personality struct {
    Openness          Unit
    Conscientiousness Unit
    Extraversion      Unit
    Agreeableness     Unit
    Neuroticism       Unit

    Sensitivity       Unit
    EmotionalInertia Unit
    RecoveryCapacity Unit
}
```

OCEAN влияет на отдельные эмоциональные каналы через конфигурацию. Нельзя использовать один глобальный personality multiplier для всего vector.

Пример:

```text
extraversion  -> усиливает положительное social excitation
neuroticism   -> усиливает fear/sadness/anger sensitivity
agreeableness -> усиливает trust и уменьшает hostility gain
openness      -> усиливает surprise/anticipation response
conscientiousness -> влияет на recovery, stability и response to failure
```

Значения и направления влияния задаются конфигурацией и проходят semantic validation.

### 5.2. Physiology

```go
type Physiology struct {
    Fatigue    Unit
    Arousal    Unit
    Energy     Unit
    StressLoad Unit
    Stability  Unit
}
```

Physiology не сводится к общему множителю.

Примеры channel-specific effects:

- высокая fatigue уменьшает gain радости и удивления;
- высокая fatigue увеличивает emotional inertia;
- высокая fatigue может уменьшать inhibition злости;
- высокий stress load усиливает fear и anger response;
- низкая energy уменьшает доступную амплитуду активных положительных состояний;
- низкая stability усиливает чувствительность к конфликтующим стимулам;
- arousal влияет на скорость возбуждения, но не определяет valence.

### 5.3. Basic emotion vector

Используются восемь базовых эмоций Плутчика:

```go
type EmotionVector struct {
    Joy          Unit
    Trust        Unit
    Fear         Unit
    Surprise     Unit
    Sadness      Unit
    Disgust      Unit
    Anger        Unit
    Anticipation Unit
}
```

Каждая эмоция имеет собственную динамику:

```go
type EmotionDynamics struct {
    Baseline          Unit
    ExcitationGain    NonNegative
    RecoveryRate      NonNegative
    Persistence       Unit
    Ceiling           Unit
    FatigueSensitivity SignedUnit
    StressSensitivity  SignedUnit
    ArousalSensitivity SignedUnit
}
```

Дополнительно профиль содержит cross-emotion interactions:

```go
type Interaction struct {
    From   Emotion
    To     Emotion
    Weight SignedUnit
    Mode   InteractionMode // excite | inhibit
}
```

Это позволяет моделировать не только четыре жёсткие пары, но и вторичные связи.

### 5.4. Relationship

Relationship state изолирован по `identity_id + user_id`.

```go
type RelationshipState struct {
    Attachment       Unit
    Openness         Unit
    Tension          Unit
    ConfidenceInUser Unit
    PerceivedSafety  Unit
    UnresolvedHurt   Unit
}
```

Relationship изменяется медленнее basic emotions и модифицирует восприятие стимула от конкретного пользователя.

Одинаковая фраза от доверенного и недоверенного пользователя может создавать разные effective stimuli.

### 5.5. Drives

Drive является внутренним направлением мотивации, а не эмоцией.

```go
type DriveKind string

type DriveState struct {
    Level        Unit
    Satisfaction Unit
    Urgency      Unit
}

type DriveDefinition struct {
    Baseline        Unit
    GrowthRate      NonNegative
    SatisfactionMap map[StimulusKind]SignedUnit
    EmotionEffects  map[Emotion]SignedUnit
}
```

Начальные drives mini MVP:

- cognition/curiosity;
- social connection;
- safety;
- coherence/control;
- recovery/rest.

Неудовлетворённый drive может повышать чувствительность к связанным стимулам. Удовлетворение drive может временно возбуждать целевые эмоции и снижать urgency.

Drives не дают движку права самостоятельно выполнять actions.

### 5.6. Complex states

Complex state является долговременным режимом динамики.

```go
type ComplexState struct {
    Kind        ComplexStateKind
    Activation  Unit
    ActiveSince time.Time
    Evidence    EvidenceAccumulator
}
```

Начальные состояния:

- depressive;
- euphoria;
- chronic_stress;
- emotional_exhaustion;
- guarded_attachment.

Это не медицинские диагнозы. Названия описывают внутренние simulation modes и не должны выводиться пользователю как клиническое заключение.

### 5.7. Temporal evidence

Сложные состояния нельзя надёжно вычислять только по последнему snapshot или среднему значению.

Нужны bounded accumulators:

```go
type EvidenceAccumulator struct {
    PositiveArea float64
    ViolationArea float64
    ObservedFor  time.Duration
    LastUpdatedAt time.Time
}
```

Evidence хранит агрегированную временную информацию без raw text.

## 6. Типизированные числовые примитивы

В реализации разрешены generics для общих bounded numeric operations:

```go
type Float interface {
    ~float32 | ~float64
}

type Bounded[T Float] struct {
    value T
}
```

Но domain model должна сохранять явные типы и имена. Универсальная математика не должна превращать `Joy`, `Fatigue`, `Trust` и `StressLoad` в неразличимые map entries.

Минимальные типы:

```go
type Unit float64        // [0, 1]
type SignedUnit float64  // [-1, 1]
type NonNegative float64 // [0, +inf)
```

Конструкторы валидируют диапазон. Прямые unchecked conversions внутри application code запрещены.

## 7. Stimulus model

Canonical stimulus остаётся typed event:

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

Stimulus не содержит готовый emotional vector.

Extractor может создать несколько stimuli из одного события. Например, apology может одновременно:

- уменьшить tension;
- увеличить trust;
- уменьшить unresolved hurt;
- создать небольшой joy response;
- частично удовлетворить drive social connection.

## 8. Transition function

Основная функция должна быть чистой относительно входных данных:

```go
func Transition(
    previous State,
    stimuli []Stimulus,
    now time.Time,
    profile Profile,
) (next State, log TransitionLog, err error)
```

При одинаковых `previous`, `stimuli`, `now` и `profile` результат обязан совпадать побитово либо в пределах явно зафиксированной float tolerance.

Порядок перехода фиксирован:

```text
1. validate version and timestamps
2. advance elapsed-time dynamics
3. update drive urgency and physiology
4. calculate effective stimuli
5. apply direct excitation
6. apply cross-emotion excitation/inhibition
7. resolve opposition and saturation
8. update relationship
9. update temporal evidence
10. enter, sustain or exit complex states
11. apply complex-state feedback
12. clamp and validate all invariants
13. increment version
14. build compact report projection
```

Порядок не зависит от iteration order map. Все domain collections с влиянием на результат обходятся в стабильном порядке.

## 9. Effective stimulus

Для канала эмоции `e`:

```text
effective(e) =
    base_weight(e, stimulus.kind)
  * stimulus.intensity
  * stimulus.confidence
  * personality_response(e)
  * physiology_response(e)
  * relationship_response(e)
  * drive_response(e)
  * complex_state_response(e)
```

Окончательный delta дополнительно ограничивается:

```text
delta(e) = clamp(
    effective(e) + cross_excitation(e) - inhibition(e),
    -max_negative_delta(e),
    +max_positive_delta(e)
)
```

Модификаторы должны быть bounded. Никакая комбинация коэффициентов не может вывести состояние за `[0, 1]` или создать NaN/Inf.

## 10. Elapsed-time dynamics

Отдельный background timer не нужен. State продвигается лениво при чтении или записи.

Для независимого восстановления используется аналитическое exponential approach:

```text
value(t + dt) = target + (value(t) - target) * exp(-rate * dt)
```

`target` не всегда равен baseline:

- complex state может временно изменить target;
- physiology может изменить effective recovery rate;
- active drive может создать медленное внутреннее excitation;
- relationship имеет собственные медленные rates.

Для coupled nonlinear effects применяется deterministic operator splitting с ограниченным числом подшагов. Размер подшага является частью versioned profile configuration.

Нельзя выполнять неограниченный цикл по каждой минуте прошедшего времени. Большой `dt` должен обрабатываться bounded способом.

## 11. Excitation, inhibition and opposition

Opposition pairs сохраняются как начальная структура:

```text
joy <-> sadness
trust <-> disgust
fear <-> anger
surprise <-> anticipation
```

Но правило не должно просто обнулять слабейшую эмоцию после каждого update.

Используется конфигурируемая inhibition function:

```text
suppression(to) =
    source_value
  * pair_weight
  * source_dominance
  * target_susceptibility
```

Mixed states допустимы. Система должна разрешать, например:

- joy + fear;
- trust + sadness;
- anger + attachment;
- anticipation + guardedness.

Запрещено только неограниченное одновременное насыщение конфликтующих каналов.

## 12. Fatigue and physiology effects

Fatigue должна иметь самостоятельную динамику:

```text
fatigue increases from:
- long cognitive run
- repeated provider/tool failure
- prolonged high arousal
- chronic stress
- insufficient recovery interval

fatigue decreases from:
- elapsed quiet time
- recovery/rest drive satisfaction
- low arousal period
```

Пример воздействия высокой fatigue:

```text
joy excitation gain       decreases
surprise excitation gain  decreases
anger inhibition control  decreases
emotion persistence       increases
recovery capacity         decreases
stability                  decreases
```

Конкретные коэффициенты задаются profile config и тестируются сценариями.

## 13. Complex-state activation

Complex-state recipe содержит:

```go
type ComplexStateDefinition struct {
    EntryConditions []Condition
    ExitConditions  []Condition
    MinEntryDuration time.Duration
    MinExitDuration  time.Duration
    EntryThreshold   Unit
    ExitThreshold    Unit
    Effects          StateEffects
}
```

Обязательные свойства:

- отдельные entry и exit thresholds;
- hysteresis;
- минимальная длительность входа;
- минимальная длительность восстановления;
- отсутствие активации при недостаточных данных;
- сохранение evidence между запросами;
- versioned definition ID в state metadata.

## 14. Depression feedback example

Начальная simulation recipe для `depressive`:

Entry evidence может учитывать:

- длительно повышенную sadness;
- длительно пониженную joy;
- высокую fatigue;
- низкую energy;
- низкую social-drive satisfaction;
- отсутствие устойчивого положительного recovery.

После активации состояние меняет динамику:

```text
joy excitation gain        *= configured value below 1
joy effective ceiling      decreases
sadness recovery rate      *= configured value below 1
fatigue recovery rate      *= configured value below 1
negative stimulus gain     may increase
social withdrawal bias     increases
```

Выход требует устойчивого восстановления, а не одной вспышки радости.

Эта схема намеренно создаёт обратную связь, но все coefficients bounded, чтобы состояние оставалось обратимым и тестируемым.

## 15. Complex states are not labels only

Запрещён вариант реализации, где complex state только добавляется в `Report` и не участвует в следующем `Transition`.

Каждое active state обязано иметь одно или несколько typed effects:

- target shift;
- gain modifier;
- recovery modifier;
- ceiling modifier;
- inhibition modifier;
- drive modifier;
- report bias.

## 16. Relationship and per-user isolation

Canonical key:

```text
state_key = sonata_identity_id + user_id
```

В mini MVP:

- нет global emotional state;
- нет переноса отношений между пользователями;
- один пользователь не влияет на complex states другого;
- event history и accumulators строго owner-scoped;
- optimistic version conflict решается повторным deterministic replay.

## 17. Persistence model

Canonical persistence в Neon должна хранить:

```text
affective_states
- identity_id
- user_id
- version
- profile_version
- emotion vector
- physiology
- relationship
- drives
- active complex states
- temporal accumulators
- last_updated_at

affective_events
- event_id
- identity_id
- user_id
- state_version_before
- state_version_after
- stimulus kind
- bounded numeric attributes
- safe audit metadata
- created_at
```

Raw message text не дублируется.

Event log должен позволять восстановить state replay или обнаружить рассогласование materialized snapshot.

## 18. Reporting and cognitive boundary

Affective engine создаёт один canonical report version, после чего application layer строит role-specific projections.

Router не получает EmotionReport.

В mini MVP прямой report разрешён:

- Raw prisms;
- Critical phase;
- Synthesis tooling;
- Synthesis final.

Summary phase не получает отдельный report, пока это не будет принято отдельным contract change. Она обобщает raw и critical output своей призмы.

Все разрешённые роли одного cognitive run должны видеть одну и ту же `StateVersion`.

Report содержит только необходимые biases:

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

Report не содержит полный event history, raw text, protected instructions или внутренние диагностические коэффициенты.

## 19. LLM boundary

Обязательный путь:

```text
structured backend event or deterministic text extractor
-> typed Stimulus
-> validation
-> Transition
-> versioned State
-> compact Report
```

Необязательный будущий путь:

```text
LLM classifier
-> untrusted EmotionHint
-> schema validation
-> confidence threshold
-> conversion to bounded Stimulus
-> Transition
```

LLM output никогда не записывается как готовый state.

## 20. Graceful degradation

При повреждённом state или storage failure:

```text
load validated baseline profile
-> create degraded report
-> continue cognitive pipeline
-> do not fabricate event history
```

Degraded mode не должен незаметно перезаписывать canonical state. Recovery выполняется отдельной repository operation с audit record.

## 21. Verification strategy

### 21.1. Invariants

Для каждого transition проверяются:

- все bounded values находятся в допустимом диапазоне;
- нет NaN и Inf;
- version увеличивается ровно на один;
- timestamp не движется назад;
- owner key неизменяем;
- deterministic order не зависит от map iteration;
- protected/security state отсутствует в domain model.

### 21.2. Golden trajectories

Нужны versioned сценарии:

- warmth and trust accumulation;
- hostility followed by apology;
- fatigue after long cognitive load;
- recovery after quiet interval;
- prolonged sadness without depression;
- entry into depressive state;
- suppressed joy while depressive state is active;
- gradual recovery and hysteresis-based exit;
- conflicting fear and anger;
- same stimulus under two different personalities;
- same stimulus from trusted and untrusted user;
- replay after optimistic lock conflict.

### 21.3. Property and fuzz tests

Проверяются:

- arbitrary valid event sequences never break bounds;
- elapsed time never creates non-finite values;
- partitioning one elapsed interval into smaller intervals stays within accepted numerical tolerance;
- replay yields the same final state;
- cross-user events never modify another owner state;
- malformed metadata and secret-like keys are rejected.

### 21.4. Long-horizon simulation

Нужны симуляции на дни и недели виртуального времени без real sleep.

Проверяется:

- convergence to baseline in neutral conditions;
- отсутствие runaway feedback;
- возможность выйти из каждого complex state;
- отсутствие oscillation около threshold благодаря hysteresis;
- bounded runtime для большого elapsed interval.

## 22. Implementation sequence

### Increment 1. Domain types

- bounded numeric primitives;
- typed Personality;
- typed Physiology;
- DriveState;
- ComplexState;
- temporal accumulators;
- expanded versioned State.

### Increment 2. Profile and configuration

- per-emotion dynamics;
- personality influence matrix;
- physiology influence matrix;
- cross-emotion interaction matrix;
- drive definitions;
- complex-state recipes;
- strict validation and stable IDs.

### Increment 3. Pure transition engine

- elapsed-time advancement;
- effective stimulus calculation;
- excitation/inhibition;
- relationship update;
- deterministic transition log.

### Increment 4. Complex states

- evidence accumulation;
- hysteresis;
- entry/exit;
- feedback effects;
- long-horizon golden tests.

### Increment 5. Storage

- Neon repository preserving CAS semantics;
- event persistence;
- snapshot/replay verification.

### Increment 6. Cognitive integration

- one state version per request;
- role-specific projections;
- Router exclusion;
- graceful degradation;
- end-to-end tests.

## 23. Migration from current `internal/emotion`

Текущий package считается `v0 core` и сохраняется как полезная основа:

- typed eight-emotion vector;
- relationship state;
- per-user key;
- stimulus types;
- CAS memory store;
- deterministic extractor;
- versioned compact report;
- baseline exponential decay.

До HTTP integration он должен быть расширен или внутренне заменён в соответствии с этим contract.

Нельзя считать stage 07 полностью завершённым только потому, что v0 core проходит tests.

## 24. Acceptance criterion

Affective dynamics engine готов к интеграции, когда:

- одинаковые versioned events дают воспроизводимое состояние;
- personality, physiology и relationship действительно меняют реакцию;
- разные эмоции имеют разные excitation/recovery mechanics;
- fatigue оказывает channel-specific effects;
- drives участвуют в динамике без автономных actions;
- complex states используют duration, evidence и hysteresis;
- active complex states меняют последующие transitions;
- depressive state подавляет joy response согласно versioned config;
- все состояния обратимы и bounded;
- long-horizon, race, property и replay tests проходят;
- модуль не зависит от LLM;
- Router не получает emotional context;
- один canonical state version безопасно проецируется разрешённым cognitive roles.
