# Troubleshooting

labctl's errors say what went wrong and print the command that fixes it. This
page collects them by symptom. `labctl doctor` checks your machine, and
`labctl status` checks the lab.

## Installing and building the lab

| Symptom | Cause and fix |
|---|---|
| `labctl: command not found` after installing | `~/.local/bin` is not on your `PATH`. Add `export PATH="$HOME/.local/bin:$PATH"` to `~/.bashrc` or `~/.zshrc` and open a new terminal. |
| `could not find your lab` | labctl does not know where your clone is. Run `./install.sh` in your clone once (it records it), `cd` into the clone, or set `SNOWOPS_LAB_DIR`. |
| `the lab recorded in ~/.snowops/lab-dir … is gone` | You moved or deleted your clone. Run `./install.sh` in its new place. |
| `labctl is X but the lab in … is Y` | Your clone moved to another release (for example after `git merge origin/stable`). Run `./install.sh` in the clone to get the matching labctl. |
| `… is development content` | You cloned `main`. Clone `--branch stable`, or `git switch stable`, then `./install.sh`. |
| `install.sh`: `this checkout is development content` | Same: switch to `stable`, or build labctl from source (CONTRIBUTING). |
| `lab state is in both … and …` | An older labctl kept state inside the clone (`.labctl/`). labctl now uses `~/.snowops/state/<cluster>/`; copy what you need from `.labctl/`, then delete it. |
| `docker is too small for the lab` | Docker has less than 2 CPU / 4 GB. Resize it with the printed command (colima, Docker Desktop, `.wslconfig` or native Linux), then `labctl init`. |
| `the Docker daemon is not running` | Start it with the printed command. On macOS with colima, `labctl init` starts it for you. |
| `the Docker daemon refused this user` | You were just added to the `docker` group. Log out and back in (WSL: `wsl --shutdown` in PowerShell), then `labctl init`. |
| `this WSL distro has Docker Desktop integration turned off` | Docker Desktop → Settings → Resources → WSL Integration → enable your distro. |
| `could not download <image>` | A registry download stalled twice. Check your connection, proxy or VPN, then `labctl init` again; it resumes. |
| `the platform did not install cleanly` | Usually Docker is short of memory (`TLS handshake timeout`, pods Pending). `labctl doctor`, then `labctl init`. Installs are safe to repeat. |
| `setup-tools`: a tool `is older than` its minimum after installing | An older copy earlier on your `PATH` shadows the new one. Remove it, or put `~/.local/bin` (Homebrew's directory on macOS) earlier on `PATH`. |

## Using the lab

| Symptom | Cause and fix |
|---|---|
| The dashboard or `labctl status` says the cluster is **unreachable** | Docker or colima is stopped, usually after a reboot, or out of memory. Run `labctl init`; it keeps your apps and scenarios. |
| A node stays `NotReady` after a restart | `labctl init` restarts the cluster and corrects the address Kubernetes records for any node whose container came back with a different IP (the server included), keeping its pods. If it still fails, `labctl reset` rebuilds the lab from scratch. |
| `Not enough memory for <scenario>` | Run the commands it prints: `labctl scenario down` for a scenario, `labctl platform down` for components only it used or nothing uses. Or resize Docker as printed. [Resources](resources.md) lists what each scenario needs. |
| `<scenario> needs 2 agent nodes` on kind | kind cannot add nodes to a running cluster. Set `AGENTS=2` in `.env` and `labctl reset`. |
| `<scenario> must run on its own` | Bring down the other active scenarios first. |
| `platform prerequisite(s) not installed` or `required app(s) not deployed` | Install what it names, or re-run with `--deploy-prereqs`. |
| URLs end in `:8080` | Something else holds port 80, so the ingress moved to 8080. Every URL and check follows it. To get 80 back, free it and `labctl reset`. |
| A lab URL shows another page, an empty reply, or nothing | Another lab or program on this machine (another user's Docker VM, for one) answers on the lab's port. `labctl init` checks this and moves a k3d lab to a free port; on kind, free the port or set `HTTP_PORT` / `HTTPS_PORT` and `labctl reset`. |
| URLs end in `.k3d.local`, need `/etc/hosts` entries, or take 5 seconds to open on a Mac | The lab was built before URLs moved to `*.localhost` and keeps its names (macOS sends `.local` lookups to Bonjour, which is slow). `labctl hosts add` covers it; `labctl reset` rebuilds it with `*.localhost` names, which need nothing. |
| `a kind cluster named 'snowops' already exists` (or k3d) | Lab names are unique per machine, because URLs and lab state are keyed by them. Set `CLUSTER_NAME` in `.env` to another name. |
| A check says Prometheus or Grafana returned 404 | Another service on the lab's port answered instead of the lab. Re-run `labctl init`; it confirms the lab answers and moves it if not. |
| Grafana's Loki or Tempo queries answer `Unable to find datasource plugin` | The lab's Grafana was installed before the fix for Grafana 13's plugin install on a read-only filesystem. Reinstall it: `labctl platform up monitoring/grafana`. |
| An app pod is in `ImagePullBackOff` | Its image was not built into the cluster. `labctl app build <name>`, or use `--deploy-prereqs`. |

## Windows (WSL2)

| Symptom | Fix |
|---|---|
| `labctl ui` does not open a browser | Open `http://localhost:3939` in your Windows browser. labctl tries `wslview`, then `powershell.exe`; if WSL interop is off, neither can reach Windows. |
| `*.k3d.local` hostnames work with `curl` in WSL but not in the Windows browser | The lab was built before URLs moved to `*.localhost`. A Windows browser reads the Windows hosts file, so either add the same lines to `C:\Windows\System32\drivers\etc\hosts` as Administrator, or `labctl reset` for `*.localhost` names. |
| Scripts fail with `$'\r': command not found` | The clone is on a Windows drive, or was made with Git for Windows. Clone again inside WSL, in your Linux home (`cd ~`), with the distro's `git`, then `./install.sh`. |
| Docker does not start after `wsl --shutdown` | WSL runs without systemd. Enable it in `/etc/wsl.conf` or run `sudo service docker start`. |
| `Docker Desktop is installed on Windows but not running` (or Rancher Desktop) | Start it on Windows, reopen the terminal, `labctl init`. To use Docker Engine inside WSL instead, keep the Windows app stopped (or uninstall it) and run `SNOWOPS_NATIVE_DOCKER=1 labctl init`. |
| The lab is unreachable after the laptop slept, or after every WSL terminal was closed | WSL stopped, and the cluster with it. Run `labctl init`. If it still fails, `labctl reset` rebuilds it; it removes the old cluster's containers even when k3d can no longer read them. |
| The lab is slow or short of memory, and `docker ps` shows `k3d-…` containers of another cluster | An older k3d or kind cluster (from another project, or an older lab name) restarts with Docker and shares WSL's memory. List them with `docker ps --filter name=k3d-` (k3d may not list a half-broken one), and stop what you do not need with `k3d cluster stop <name>`, or `docker stop` on its containers. |

## Still stuck

`labctl runs list` shows every operation labctl ran and `labctl runs logs <id>`
its full output. Open an [issue](https://github.com/sagar2395/snowopslabs/issues)
with the output of `labctl doctor` and `labctl status`.
