# Agent documentation routing

## Mini MVP tasks

When a task concerns the first deployable mini MVP, start with:

```text
docs/mini-mvp/README.md
```

Follow only the documents linked from that index unless the task explicitly requires historical context or the long-term architecture.

Do not infer mini MVP requirements from `docs/stage-*` or `.artifacts` when they conflict with `docs/mini-mvp/`.

## Long-term Sonata tasks

For work on the complete future architecture, use the staged documentation under:

```text
docs/stage-*
docs/00-shared/
```

Do not automatically add long-term features to the mini MVP.

## Historical materials

Files under `.artifacts/` and donor reports describe previous implementations or migration sources. They are not current runtime specifications unless an active document explicitly references them.

## Documentation placement

- Mini MVP documents belong only in `docs/mini-mvp/`.
- Long-term architecture belongs in the staged documentation tree.
- Shared principles belong in `docs/00-shared/` only when they apply to both scopes.
