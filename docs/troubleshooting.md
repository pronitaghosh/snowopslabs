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
| `no such host` for `*.k3d.local` | Run `labctl hosts add` (asks for sudo), and again when `scenario up` reports a new hostname. |
| A check says Prometheus or Grafana returned 404 | Another service on port 80 answered instead of the lab. Re-run `labctl init` so labctl picks up the lab's real port. |
| An app pod is in `ImagePullBackOff` | Its image was not built into the cluster. `labctl app build <name>`, or use `--deploy-prereqs`. |

## Windows (WSL2)

| Symptom | Fix |
|---|---|
| `labctl ui` does not open a browser | `sudo apt install wslu`, or open `http://localhost:3939` in your Windows browser. |
| Lab hostnames work with `curl` in WSL but not in the Windows browser | The Windows browser reads the Windows hosts file. Add the same lines to `C:\Windows\System32\drivers\etc\hosts` as Administrator. |
| Scripts fail with `$'\r': command not found` | The clone is on a Windows drive, or was made with Git for Windows. Clone again inside WSL, in your Linux home (`cd ~`), with the distro's `git`, then `./install.sh`. |
| Docker does not start after `wsl --shutdown` | WSL runs without systemd. Enable it in `/etc/wsl.conf` or run `sudo service docker start`. |

## Still stuck

`labctl runs list` shows every operation labctl ran and `labctl runs logs <id>`
its full output. Open an [issue](https://github.com/sagar2395/snowopslabs/issues)
with the output of `labctl doctor` and `labctl status`.
