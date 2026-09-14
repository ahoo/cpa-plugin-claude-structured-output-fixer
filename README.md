# cpa-plugin-claude-structured-output-fixer

Legacy CLIProxyAPI request normalizer plugin for prompt Stop-hook structured
output on the claude→codex translation edge.

> [!IMPORTANT]
> CLIProxyAPI `v7.2.150` and later handle this normalization natively. This
> plugin is only for legacy hosts older than `v7.2.150` and is not needed on
> current releases.

When a prompt Stop hook requests structured output with
`text.format = {type: "json_schema", name: "cli_proxy_structured_output",
strict: true}` and the schema has optional properties missing from
`required`, strict backends reject the translated request with HTTP 400
(OpenAI strict mode requires every declared property to be required).
This plugin downgrades `strict` to `false` for exactly that shape, so the
request passes instead of failing.

## Capability

- `request_normalizer` — only fires on `FromFormat=claude`,
  `ToFormat=codex` payloads carrying the `cli_proxy_structured_output`
  json_schema with `strict: true` and a required/properties mismatch.
- Everything else passes through untouched.

## Legacy host installation

Use this only with CLIProxyAPI versions older than `v7.2.150`:

```yaml
plugins:
  enabled: true
  configs:
    claude-structured-output-fixer:
      enabled: true
      priority: 0
```

Then restart CLIProxyAPI and verify registration:

```bash
docker restart cli-proxy-api
docker logs cli-proxy-api | grep structured-output
```

## Build

Debian/glibc toolchain only (musl `.so` fails to `dlopen`):

```bash
./build.sh
```

## Upstream status

This plugin worked around a host translator gap: the claude→codex request
translator forwarded `text.format.strict: true` verbatim while the generated
schema did not satisfy strict requirements. CLIProxyAPI fixed this natively in
`v7.2.150`; current releases no longer require the plugin.

The plugin remains available only for legacy hosts older than `v7.2.150`. It is
not submitted to the official plugin store, and no new release is planned for
this documentation update.

## Test

```bash
go vet . && go test ./...
```

## License

MIT
