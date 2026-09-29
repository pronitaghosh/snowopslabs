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

Clone in your Linux home directory (`cd ~`), not under `/mnt/c`, and with the
distro's `git`, not Git for Windows. Windows drives are much slower from WSL,
and a checkout made from Windows can get Windows line endings, which break
every shell script. `labctl doctor` warns when your lab is on a Windows drive.

## Install and build the lab

In your distro's terminal:

```bash
cd ~
git clone --branch stable https://github.com/sagar2395/snowopslabs.git
cd snowopslabs
./install.sh
labctl init
```

What `init` tells you, and what to do:

| Message | Do this |
|---|---|
| Docker Desktop is not enabled for this WSL distro | Docker Desktop → Settings → Resources → WSL Integration → enable your distro, Apply & Restart, reopen the terminal. |
| Docker Desktop (or Rancher Desktop) is installed on Windows but not running | Start it on Windows, then reopen the terminal and run `labctl init` again. labctl will not install a second Docker engine next to it. |
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
  browser as is, and `labctl ui` opens it for you (through `powershell.exe`, or
  `wslview` if you have `wslu`).
- **Lab URLs** such as `http://grafana.snowops.localhost` open in your Windows
  browser as they are. The browser resolves `*.localhost` to Windows' own
  localhost, and WSL2 forwards Windows' localhost ports into the distro (Docker
  Desktop publishes them on Windows directly). Nothing goes in a hosts file.
- A lab built before URLs moved to `*.localhost` keeps its `*.k3d.local` names,
  which a Windows browser looks up in the Windows hosts file
  (`C:\Windows\System32\drivers\etc\hosts`), not WSL's. `labctl reset`
  rebuilds it with `*.localhost` names.

## After a restart

`wsl --shutdown` or a Windows restart stops the cluster. So does closing every
WSL terminal: once no terminal is open, WSL stops the distro after a short idle
time, and Docker and the cluster stop with it. Keep one WSL terminal open while
you use the lab (the one running `labctl ui` is enough).

When you come back, run `labctl init`; it restarts the cluster in order and
keeps your apps and scenarios.

## Uninstall

```bash
labctl teardown
rm -rf ~/.snowops ~/.local/bin/labctl   # and your clone, when you no longer need it
```
