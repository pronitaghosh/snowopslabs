# CLI reference

`labctl` is the command-line interface for SnowOps Labs. It builds the lab,
runs scenarios, injects faults and grades your fix.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/sagar2395/snowopslabs/main/install.sh | sh
```

The installer puts `labctl` in `~/.local/bin` and the lab content in
`~/.snowops/lab`, with no sudo; re-run it to upgrade. Set `SNOWOPS_VERSION` for
a specific release. The per-OS guides are under
[Getting started](../../getting-started/macos.md).

Outside a checkout, `labctl` uses the installed lab; inside a checkout it uses
the checkout's content. `--project-dir` chooses explicitly.

**Build from source** (contributors) — needs Go 1.25+ and Node 22+; see
[CONTRIBUTING](../../../CONTRIBUTING.md#development-setup):

```bash
make cli-build        # builds bin/labctl with the UI embedded
```

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--project-dir` | auto-detected | The lab content to use: a checkout, or the installed `~/.snowops/lab` |
| `-v, --verbose` | `false` | Debug logging: config load, script exec, API calls |
| `--version` | — | Print the build version |

## Command map

| Page | Commands |
|---|---|
| [Lifecycle](lifecycle.md) | `init` `teardown` `reset` `status` `doctor` `setup-tools` `check` `hosts` `runtime` `lab` `runs` |
| [Applications & services](apps.md) | `app` `service` |
| [Platform](platform.md) | `platform` |
| [Scenarios](scenarios.md) | `scenario` `validate` |
| [Incidents](incidents.md) | `incident` |
| [Traffic](traffic.md) | `traffic` |
| [Comparing stacks](compare.md) | `compare` |
| [Learning & challenges](learning.md) | `learn` `challenge` |
| [Server, metrics & auth](server.md) | `ui` `users` |

## CLI or Make

Both work. Make targets are more granular; the CLI adds scenarios, the web UI
and a unified status view.

| Operation | CLI | Make |
|---|---|---|
| Full setup | `labctl init` | `make init` |
| Build an app | `labctl app build go-api` | `make build APP_NAME=go-api` |
| Deploy an app | `labctl app deploy go-api` | `make deploy APP_NAME=go-api` |
| Platform status | `labctl platform status` | `make platform-status` |
| Activate a scenario | `labctl scenario up observability-sre` | CLI only |
| Web dashboard | `labctl ui` | CLI only |
| Deploy every app | — | `make deploy-all` |
