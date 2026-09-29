# CLI reference

`labctl` is the command-line interface for SnowOps Labs. It builds the lab,
runs scenarios, injects faults and grades your fix.

## Install

```bash
git clone --branch stable https://github.com/sagar2395/snowopslabs.git
cd snowopslabs && ./install.sh
```

`install.sh` puts the `labctl` release named by the clone's `LAB_VERSION` in
`~/.local/bin`, with no sudo, and records the clone in `~/.snowops/lab-dir`.
Re-run it after upgrading the clone (`git merge origin/stable`). The per-OS
guides are under [Getting started](../../getting-started/macos.md).

`labctl` finds the lab in this order: `--project-dir`; the clone around the
working directory; `SNOWOPS_LAB_DIR`; the recorded clone. Lab state (active
scenarios and faults, history, progress, snapshots, users) is kept per cluster
in `~/.snowops/state/<cluster>/` (`SNOWOPS_HOME` moves `~/.snowops`). State an
older labctl kept in the clone's `.labctl/` is moved there on first use.

**Build from source** (contributors) — needs Go 1.25+ and Node 22+; see
[CONTRIBUTING](../../../CONTRIBUTING.md#development-setup):

```bash
make cli-build        # builds bin/labctl with the UI embedded
```

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--project-dir` | auto-detected | The lab (a snowopslabs clone) to use; see [Install](#install) for how it is found otherwise |
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
