# Getting started on macOS

Works on Apple Silicon and Intel Macs.

## What you need

- **Homebrew.** If you don't have it, `labctl init` installs it; that is the one
  step that asks for your password.
- **2 CPUs and 4 GB of memory for Docker**, more to run several scenarios at
  once ([Resources](../resources.md)).
- **About 10 GB of free disk.**

You don't need Docker Desktop. `labctl init` installs
[colima](https://github.com/abiosoft/colima), a lightweight Docker VM, with
Homebrew and starts it at 2 CPU / 4 GB. If you already use Docker Desktop, the
lab uses it instead; give it at least 2 CPUs and 4 GB under Settings →
Resources.

## Install and build the lab

```bash
curl -fsSL https://raw.githubusercontent.com/sagar2395/snowopslabs/main/install.sh | sh
labctl init
```

`init` installs `kubectl`, `helm`, `k3d`, `colima`, `docker` and
`docker-buildx` with Homebrew (a tool you already have at a new enough version
is left alone), starts colima, creates the cluster and installs the platform.
It finishes with the lab's URLs. Then:

```bash
labctl ui                  # http://localhost:3939
labctl hosts add           # optional: open Grafana & co. by name (asks for sudo)
```

## Giving colima more memory

Set the size in `~/.snowops/lab/.env`; `labctl init` starts a *stopped* colima
at that size, and never shrinks it:

```bash
LAB_CPUS=4
LAB_MEMORY=8
```

A colima that is already running is never resized for you, because restarting
it stops your other containers. Resize it yourself, then bring the lab back:

```bash
colima stop && colima start --cpu 4 --memory 8
labctl init
```

## After a reboot

colima does not start on its own. Run `labctl init`: it starts colima at the
lab's size, restarts the cluster and waits until everything is Ready, keeping
your apps and scenarios.

## Uninstall

```bash
labctl teardown
labctl hosts remove                  # if you ran `labctl hosts add`
rm -rf ~/.snowops ~/.local/bin/labctl
colima delete --data                 # removes the VM and every image in it
```

`colima delete` without `--data` keeps Docker's images and containers for the
next VM.
