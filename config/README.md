# Sonata configuration

`index.yaml` is the only configuration entrypoint. The loader applies files in this order:

1. fail-closed code defaults;
2. `base/*` fragments in the order declared by `index.yaml`;
3. the selected environment profile in declared order;
4. logical secret references from `secrets.yaml`.

YAML anchors are allowed only inside one file. Cross-file references must use stable logical IDs such as `provider_ref`, `api_key_ref`, or `endpoint_ref`.

Validate locally:

```bash
sonata config validate --config-root config --profile local
```

Print a redacted snapshot:

```bash
sonata config print --config-root config --profile local --redacted
```

Real secret values belong in Render Environment Groups or secret files. They must never be committed.
