# Sonata mini MVP

> Эта директория является единственным каноническим источником документации по mini MVP.

Документы основной архитектуры в `docs/stage-*` описывают долгосрочную Sonata и не должны использоваться как спецификация первой развёртываемой версии, если здесь явно не дана ссылка.

## Порядок чтения

1. [`ARCHITECTURE.md`](./ARCHITECTURE.md) — граница продукта, Go-стек, когнитивный цикл, OpenWebUI, RAG, инструменты и развёртывание.
2. [`contracts/AGENT_INSTRUCTION_XML.md`](./contracts/AGENT_INSTRUCTION_XML.md) — защищённые XML-инструкции и пользовательские overlays.
3. [`modules/EMOTION_MODULE.md`](./modules/EMOTION_MODULE.md) — детерминированный эмоциональный слой Sonata.
4. [`decisions/OPENCODE_ZEN_PROVIDER.md`](./decisions/OPENCODE_ZEN_PROVIDER.md) — основной provider, общий master key и будущий BYOK.

## Правила для агентов

- При работе над mini MVP сначала читать эту директорию.
- Не переносить возможности полной версии в mini MVP без явного решения.
- Не использовать файлы из `.artifacts` как актуальную архитектурную спецификацию. Они являются источниками миграции и историческими материалами.
- Не смешивать roadmap полной Sonata с критериями готовности mini MVP.
- Новые MVP-контракты, ADR и планы размещать только внутри `docs/mini-mvp/`.

## Структура

```text
docs/mini-mvp/
├── README.md
├── ARCHITECTURE.md
├── contracts/
│   └── AGENT_INSTRUCTION_XML.md
├── modules/
│   └── EMOTION_MODULE.md
└── decisions/
    └── OPENCODE_ZEN_PROVIDER.md
```
