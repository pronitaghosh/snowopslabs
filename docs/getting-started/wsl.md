# Getting started on Windows (WSL2)

The lab runs inside WSL2, as it would on Linux. Run every command in your WSL
distro's terminal (Ubuntu is recommended), never in PowerShell.

## What you need

- **WSL2 with a Linux distro.** In PowerShell: `wsl --install -d Ubuntu`.
- **Docker, one of:**
  - **Docker Desktop** with Settings → Resources → **WSL Integration** turned
    on for your distro, or
  - **Docker Engine inside the distro**, which `labctl init` installs if
    neither is present.
- **2 CPUs and 4 GB of memory for WSL** ([Resources](../resources.md)).

## Keep the lab on the Linux filesystem

Work in your Linux home directory (`~`), not under `/mnt/c`. Windows drives are
much slower from WSL, and a checkout there can get Windows line endings, which
break every shell script. The installer already puts the lab in
`~/.snowops/lab`.

## Install and build the lab

```bash
curl -fsSL https://raw.githubusercontent.com/sagar2395/snowopslabs/main/install.sh | sh
labctl init
```

What `init` tells you, and what to do:

| Message | Do this |
|---|---|
| Docker Desktop is not enabled for this WSL distro | Docker Desktop → Settings → Resources → WSL Integration → enable your distro, Apply & Restart, reopen the terminal. |
| added to the docker group | Run `wsl --shutdown` in PowerShell, reopen the terminal, run `labctl init` again. |
| WSL is running without systemd | Docker will not start by itself. Add `[boot]` and `systemd=true` to `/etc/wsl.conf`, then `wsl --shutdown`; or run `sudo service docker start` in each session. |

## Memory

Docker's memory comes from WSL, whether you use Docker Desktop or Docker
Engine. Set it in `%UserProfile%\.wslconfig` on Windows:

```ini
[wsl2]
memory=6GB
processors=4
```

then run `wsl --shutdown` in PowerShell and reopen the terminal.

## Opening the lab from Windows

- **The dashboard** (`labctl ui`, `http://localhost:3939`) works in your Windows
  browser as is. Install `wslu` (`sudo apt install wslu`) so `labctl ui` can
  open it for you.
- **Lab hostnames** such as `grafana.k3d.local`: a Windows browser reads the
  Windows hosts file, not WSL's. Add the lines `labctl hosts add` writes to
  `C:\Windows\System32\drivers\etc\hosts` (as Administrator). If WSL rewrites
  its own `/etc/hosts` on restart, set `generateHosts=false` under `[network]`
  in `/etc/wsl.conf`.

## After a restart

`wsl --shutdown` or a Windows restart stops the cluster. Run `labctl init`; it
restarts the cluster in order and keeps your apps and scenarios.

## Uninstall

```bash
labctl teardown
rm -rf ~/.snowops ~/.local/bin/labctl
```
