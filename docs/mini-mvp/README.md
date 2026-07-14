# Sonata mini MVP

> Эта директория является единственным каноническим источником документации по mini MVP.

Документы основной архитектуры в `docs/stage-*` описывают долгосрочную Sonata и не должны использоваться как спецификация первой развёртываемой версии, если здесь явно не дана ссылка.

## Порядок чтения

1. [`CHECKLIST.md`](./CHECKLIST.md) — рабочий порядок реализации и критерии приёмки mini MVP.
2. [`ARCHITECTURE.md`](./ARCHITECTURE.md) — продуктовая граница и когнитивная архитектура mini MVP.
3. [`TECH_STACK.md`](./TECH_STACK.md) — выбранные технологии, библиотеки, модели, deployment и observability.
4. [`contracts/INSTRUCTION_AND_MANIFEST.md`](./contracts/INSTRUCTION_AND_MANIFEST.md) — неизменяемые instructions и переключаемые manifests.
5. [`contracts/OPENWEBUI_BOUNDARY.md`](./contracts/OPENWEBUI_BOUNDARY.md) — service credential, forwarded identity и SSE boundary.
6. [`deployment/RENDER_OPENWEBUI.md`](./deployment/RENDER_OPENWEBUI.md) — Render Blueprint, private Sonata API и live acceptance OpenWebUI.
7. [`modules/EMOTION_MODULE.md`](./modules/EMOTION_MODULE.md) — границы и базовая интеграционная роль emotional layer.
8. [`modules/AFFECTIVE_DYNAMICS.md`](./modules/AFFECTIVE_DYNAMICS.md) — принятая математическая и доменная спецификация affective dynamics engine; имеет приоритет над упрощённой transition-моделью `EMOTION_MODULE.md`.
9. [`modules/RELATIONSHIP_RESPONSE.md`](./modules/RELATIONSHIP_RESPONSE.md) — bounded и versioned правило влияния отношений с пользователем на последующие emotional responses.
10. [`modules/AFFECTIVE_GOLDEN_TRAJECTORIES.md`](./modules/AFFECTIVE_GOLDEN_TRAJECTORIES.md) — versioned verification contract и semantic/numeric trajectories stage 07B.
11. [`modules/AFFECTIVE_REPORT_PROJECTION.md`](./modules/AFFECTIVE_REPORT_PROJECTION.md) — единая report version и разрешённая projection topology cognitive roles.
12. [`modules/AFFECTIVE_HTTP_INTEGRATION.md`](./modules/AFFECTIVE_HTTP_INTEGRATION.md) — owner-scoped affective transition внутри trusted HTTP chat flow и graceful degradation.
13. [`decisions/OPENCODE_ZEN_PROVIDER.md`](./decisions/OPENCODE_ZEN_PROVIDER.md) — основной provider, общий master key и будущий BYOK.
14. [`decisions/CONFIG_AND_SECRETS.md`](./decisions/CONFIG_AND_SECRETS.md) — единая загрузка конфигурации и секретов.
15. [`decisions/NEON_CANONICAL_STORAGE.md`](./decisions/NEON_CANONICAL_STORAGE.md) — schema `sonata`, pooled/direct connections, owner constraints, migrations и affective CAS repository.

## Правила для агентов

- При работе над mini MVP сначала читать `CHECKLIST.md`, затем только связанные с текущим этапом документы.
- После реализации или отмены требования обновлять checkbox в `CHECKLIST.md` в том же изменении.
- Не отмечать пункт выполненным без проверки указанного критерия приёмки.
- Не переносить возможности полной версии в mini MVP без явного решения.
- Не использовать файлы из `.artifacts` как актуальную архитектурную спецификацию. Они являются источниками миграции и историческими материалами.
- Не смешивать roadmap полной Sonata с критериями готовности mini MVP.
- Новые MVP-контракты, ADR и планы размещать только внутри `docs/mini-mvp/`.
- При конфликте документов приоритет имеет более узкий contract или accepted ADR.
- User manifest никогда не заменяет protected instruction.
- Реальные secret values никогда не хранятся в YAML репозитория.

## Структура

```text
docs/mini-mvp/
├── README.md
├── CHECKLIST.md
├── ARCHITECTURE.md
├── TECH_STACK.md
├── contracts/
│   ├── INSTRUCTION_AND_MANIFEST.md
│   └── OPENWEBUI_BOUNDARY.md
├── deployment/
│   └── RENDER_OPENWEBUI.md
├── modules/
│   ├── EMOTION_MODULE.md
│   ├── AFFECTIVE_DYNAMICS.md
│   ├── RELATIONSHIP_RESPONSE.md
│   ├── AFFECTIVE_GOLDEN_TRAJECTORIES.md
│   ├── AFFECTIVE_REPORT_PROJECTION.md
│   └── AFFECTIVE_HTTP_INTEGRATION.md
└── decisions/
    ├── OPENCODE_ZEN_PROVIDER.md
    ├── CONFIG_AND_SECRETS.md
    └── NEON_CANONICAL_STORAGE.md
```
