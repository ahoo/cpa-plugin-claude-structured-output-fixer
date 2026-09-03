# cpa-plugin-claude-structured-output-fixer

CLIProxyAPI request normalizer plugin for prompt Stop-hook structured output
on the claude→codex translation edge.

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

## Install

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

## Upstream note

This is a workaround for a host translator gap: the claude→codex request
translator forwards `text.format.strict: true` verbatim while the generated
schema does not satisfy strict requirements. A proper host-side fix would
complete `required` (or drop `strict`) during translation. Until then, this
plugin keeps Stop-hook structured output working. **Not submitted to the
official plugin store** — tracked for removal once the host fixes it.

## Test

```bash
go vet . && go test ./...
```

## License

MIT
