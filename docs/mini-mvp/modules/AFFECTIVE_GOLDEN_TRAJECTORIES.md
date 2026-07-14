# Affective golden trajectories

> Статус: verification contract для stage 07B  
> Profile: `sonata-affective-v1.0.0`  
> Основной design contract: [`AFFECTIVE_DYNAMICS.md`](./AFFECTIVE_DYNAMICS.md)  
> Relationship rule: [`RELATIONSHIP_RESPONSE.md`](./RELATIONSHIP_RESPONSE.md)

## 1. Назначение

Golden trajectories фиксируют не отдельную формулу или локальный unit test, а наблюдаемое поведение affective engine на последовательности versioned событий.

Они нужны, чтобы изменения коэффициентов, порядка операторов, recovery logic, drives и complex-state feedback не проходили незаметно под видом безопасного рефакторинга.

Для mini MVP используются два уровня проверки:

1. **Semantic trajectory** — проверяет направление изменения, bounds, версии, ownership и воспроизводимость.
2. **Numeric snapshot** — фиксирует выбранные числовые значения состояния для конкретной версии профиля.

Semantic trajectories обязательны до HTTP integration. Numeric snapshots добавляются после подтверждённого запуска полного test suite и не заменяют property/fuzz tests.

## 2. Неподвижные правила

- Каждый golden scenario явно привязан к `profile_version`.
- Изменение ожидаемой траектории требует либо новой версии профиля, либо отдельного принятого contract change.
- Тесты не используют real sleep, LLM, provider, tools, storage или network.
- Одинаковые `previous state + stimuli + now + profile` должны давать одинаковый state и `TransitionLog`.
- Проверки не должны зависеть от map iteration order.
- Все значения после каждого шага должны проходить `AffectiveState.Validate()`.
- Golden tests не могут подменять fuzz, property, race и long-horizon проверки.

## 3. Реализованные semantic trajectories

### 3.1. Warmth and trust accumulation

Файл: `internal/emotion/affective_golden_v1_test.go`

Последовательность:

```text
baseline
-> user_warmth
-> user_trust
```

Проверяется:

- рост joy и trust;
- накопление attachment, openness и perceived safety;
- рост confidence in user после trust stimulus;
- привязка теста к `sonata-affective-v1.0.0`.

### 3.2. Hostility followed by apology

Последовательность:

```text
baseline
-> user_hostility
-> user_apology
```

Проверяется:

- hostility увеличивает anger и уменьшает trust;
- apology уменьшает anger;
- apology увеличивает trust и openness;
- tension и unresolved hurt снижаются;
- stress load снижается, stability повышается.

### 3.3. Repeated tool failure and quiet recovery

Последовательность:

```text
baseline
-> tool_failure
-> tool_failure
-> tool_failure
-> 24h quiet interval
```

Проверяется:

- накопление fatigue и stress load;
- восстановление physiology после quiet interval;
- bounded integration substeps.

### 3.4. Mixed fear and anger

Последовательность:

```text
baseline
-> user_distress
-> user_hostility
```

Проверяется:

- fear и anger могут одновременно оставаться активными;
- opposition/inhibition не сводит mixed state к принудительному winner-takes-all;
- результат остаётся bounded и валидным.

### 3.5. Same stimulus under supported and strained relationship

Файлы:

- `internal/emotion/affective_relationship_response_test.go`;
- `internal/emotion/affective_relationship_trajectory_test.go`.

Правило: `relationship-response-v1` для профиля `sonata-affective-v1.0.0`.

Последовательности:

```text
supported relationship -> user_warmth
strained relationship  -> user_warmth

supported relationship -> user_hostility
strained relationship  -> user_hostility
```

Проверяется:

- одинаковый `user_warmth` создаёт больший joy и trust delta при высоком support;
- одинаковый `user_hostility` создаёт меньший anger и disgust delta при высоком support и низком strain;
- high support смягчает отрицательный trust delta;
- sign-aware rule зеркалит отрицательные effects относительно neutral modifier `1.0`;
- relationship effects текущего stimulus не меняют его собственный emotional response;
- повтор transition даёт идентичные state и `TransitionLog`;
- `TransitionLog.RelationshipRule` фиксирует применённый rule ID.

**Verification status:** full repository test suite и GitHub CI проходят на head `7ac6dba38f3652969f6eb946f48689b462c44250`. Relationship-response semantic trajectories приняты, increment закрыт.

## 4. Уже покрытые соседними tests сценарии

Следующие accepted scenarios проверяются существующими файлами и не должны дублироваться отдельной копией без причины:

- deterministic ordering и повторяемость transition — `affective_transition_test.go`;
- same stimulus under different personalities — `affective_transition_test.go`;
- fatigue-dependent joy response — `affective_transition_test.go`;
- suppressed joy under depressive state — `affective_transition_test.go` и `affective_transition_invariants_test.go`;
- complex-state entry, exit и hysteresis — `affective_evidence_test.go`;
- bounded large elapsed interval — `affective_evidence_test.go` и `affective_trajectory_test.go`;
- replay identical trajectory — `affective_trajectory_test.go`;
- deterministic property corpus и fuzz seeds — `affective_trajectory_test.go`.

## 5. Незакрытые trajectories

### 5.1. Replay after optimistic lock conflict

**Статус: blocked by stage 08 storage.**

Сценарий требует:

```text
load state version N
-> calculate transition
-> CAS conflict
-> reload canonical state
-> deterministic event replay
-> persist N+k
```

Он добавляется вместе с Neon repository и optimistic lock integration tests.

### 5.2. Full numeric snapshots

**Статус: ready for implementation after verified CI.**

Полный test suite и CI подтверждены на head `7ac6dba38f3652969f6eb946f48689b462c44250`. Следующий increment создаёт versioned fixture с выбранными полями:

- state version;
- emotion vector;
- physiology;
- relationship;
- drive satisfaction and urgency;
- active complex states;
- evidence accumulators;
- transition metadata, включая `RelationshipRule`.

Snapshot должен использовать явную float tolerance или стабильное decimal normalization. Сырые значения Go `%#v` не являются переносимым golden format.

## 6. Acceptance gate

Golden trajectory increment считается завершённым, когда:

- semantic scenarios проходят в CI;
- relationship-response scenario реализован и проходит;
- complex-state entry, sustain и exit подтверждены long-horizon trajectory;
- replay после CAS conflict проходит на PostgreSQL integration test;
- numeric snapshot format зафиксирован и привязан к profile version;
- изменение profile coefficients либо сохраняет snapshots, либо сопровождается осознанным version bump.
