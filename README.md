<div align="center">

# Surge

**Blazing fast TUI download manager built in Go for power users**

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/SurgeDM/Surge)
[![Release](https://img.shields.io/github/v/release/SurgeDM/Surge?style=flat-square&color=blue)](https://github.com/SurgeDM/Surge/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/SurgeDM/Surge?style=flat-square&color=cyan)](go.mod)
[![License](https://img.shields.io/badge/License-MIT-grey.svg?style=flat-square)](LICENSE)
[![BuyMeACoffee](https://raw.githubusercontent.com/pachadotdev/buymeacoffee-badges/main/bmc-violet.svg)](https://www.buymeacoffee.com/surge.downloader)
[![Stars](https://img.shields.io/github/stars/SurgeDM/Surge?style=social)](https://github.com/SurgeDM/Surge/stargazers)

[Documentation](docs/README.md) • [Installation](#installation) • [Usage](#usage) • [Themes](docs/guides/customize-surge.md) • [Fonts](docs/guides/customize-surge.md) • [Benchmarks](#benchmarks) • [Extension](#browser-extension) • [Web UI](#web-ui) • [Settings](docs/SETTINGS.md) • [CLI Reference](docs/reference/cli.md)

</div>

---

## What is Surge?

Surge is designed for power users who prefer a keyboard-driven workflow. It features a beautiful **Terminal User Interface (TUI)**, a background **Headless Server**, a **CLI tool** for automation, and a **Web UI** for managing downloads from any browser.

![Surge Demo](assets/demo.gif)

---

## Why use Surge?

Most browsers open a single connection for a download. Surge opens multiple (up to 16 by default), splits the file, and downloads chunks in parallel. But we take it a step further:

- **Blazing Fast:** Designed to maximize your bandwidth utilization and download files as quickly as possible.
- **Multiple Mirrors:** Download from multiple sources simultaneously. Surge distributes workers across all available mirrors and automatically handles failover.
- **Sequential Download:** Option to download files in strict order (Streaming Mode). Ideal for media files that you want to preview while downloading.
- **Daemon Architecture:** Surge runs a single background "engine." You can open 10 different terminal tabs and queue downloads; they all funnel into one efficient manager.
- **Beautiful TUI:** Built with Bubble Tea & Lipgloss, featuring customizable palettes and full theme engine support.
- **Web UI:** A browser client that talks to the same daemon, so you can manage downloads from your phone, a tablet, or any machine on your network.

For a deep dive into how we make downloads faster (like work stealing and slow worker handling), check out our **[Optimization Guide](docs/OPTIMIZATIONS.md)**.

---

## Support the Project

We are just two CS students building Surge in between classes and exams. We love working on this, but maintaining a project of this scale takes time and resources. That's where you come in!

If Surge saves you time, consider supporting the development! Donations go directly toward:

- **Publishing the Extension:** Paying the Chrome Web Store fee so you can finally install the extension officially (no more sideloading!).
- **Dev Tools:** Licenses for tools like **GoReleaser Pro** to help us automate our builds.
- **Debrid Integration:** Covering subscription costs so we can test and build native Debrid support.

[**☕ Buy us a coffee**](https://www.buymeacoffee.com/surge.downloader)

_Totally optional-your stars, issues, and contributions already mean the world to us! :)_

---

## Installation

Surge is available on multiple platforms. Choose the method that works best for you.

| Platform / Method                  | Command / Instructions                                                           | Notes                                        |
| :--------------------------------- | :------------------------------------------------------------------------------- | :------------------------------------------- |
| **Prebuilt Binary**          | [Download from Releases](https://github.com/SurgeDM/Surge/releases/latest) | Easiest method. Just download and run.       |
| **Linux AppImage**           | [Download from Releases](https://github.com/SurgeDM/Surge/releases/latest) | Run `chmod +x Surge_..._linux_x86_64.AppImage`, then `./Surge_..._linux_x86_64.AppImage`. Supports delta updates. |
| **Arch Linux (AUR)**         | `yay -S surge`                                                                 | Managed via AUR.                             |
| **macOS / Linux (Homebrew)** | `brew install SurgeDM/tap/surge`                                      | Recommended for Mac/Linux users.             |
| **Nix / NixOS**              | `nix run github:SurgeDM/Surge`                                        | Via Nix flake. NixOS config: `inputs.surge.packages.${pkgs.system}.default` |
| **Windows**         | `winget install surge-downloader.surge`<br />or<br />`scoop install surge`<br />or<br />`choco install surge` | Recommended for Windows users.               |
| **Dockerfile**               | [See instructions](#4-server-mode-with-docker-compose)                              | Run Surge in server mode with Docker Compose |
| **Web UI (Docker)**          | [See instructions](#web-ui)                                                          | Browser client for the daemon, on port 1777 |
| **Go Install**               | `go install github.com/SurgeDM/Surge@latest`                          | Requires Go 1.25+                           |

---

## Usage

Surge has two main modes: **TUI (Interactive)** and **Server (Headless)**.

For a full reference, see the **[customization guide](docs/guides/customize-surge.md)**, **[Settings &amp; Configuration Guide](docs/SETTINGS.md)** and the **[CLI reference](docs/reference/cli.md)**.

### 1. Interactive TUI Mode

Just run `surge` to enter the dashboard. This is where you can visualize progress, manage the queue, and see speed graphs. If you encounter any issues, press `?` to open the bug reporting wizard. You can also press `Shift + A` or use your terminal's paste shortcut to automatically parse a copied browser 'cURL' command straight into a new download.

```bash
# Start the TUI
surge

# Start the TUI without the local HTTP API server
surge --no-server

# Start TUI with downloads queued
surge https://example.com/file1.zip https://example.com/file2.zip

# Combine URLs and batch file
surge https://example.com/file.zip --batch urls.txt
```

`--no-server` keeps the TUI fully local and skips the embedded HTTP API. CLI control commands such as `surge add`, `surge pause`, and browser-extension requests will not be able to target that instance.

### 2. Server Mode (Headless)

Great for servers, Raspberry Pis, or background processes.

```bash
# Start the server
surge server

# Start the server with a download
surge server https://url.com/file.zip

# Start with explicit API token
surge server --token <token>
```

### 3. Auto-Start Service

Surge provides an official way to manage it as a system service (daemon). This is the recommended way for servers and reproducible deployments.

```bash
# Install Surge as a system service
surge service install

# Manage the service
surge service start
surge service stop
surge service status
surge service token

# Uninstall the service
surge service uninstall
```

> [!NOTE]
> On Linux, these commands may require `sudo`. On Windows, they should be run in an elevated (Administrator) terminal.

#### Reinstalling the service after a code change (Linux + SELinux)

On SELinux-enforcing distros such as **Bazzite** or Fedora Atomic, a system
service can only execute a binary that lives in a system path. A binary in your
home directory is labelled `user_home_t`, and systemd fails it with
`status=203/EXEC` (`Permission denied`). Symlinking it into `/usr/local/bin`
does **not** help, because systemd resolves the link back to the home-directory
file. After changing the code, rebuild and redeploy a real copy, then refresh
the unit:

```bash
cd ~/Code/Surge
go build -o surge .

# Copy a real file (not a symlink) into a system path so it gets the bin_t label
sudo install -m 0755 ./surge /usr/local/bin/surge
sudo restorecon -v /usr/local/bin/surge

# Refresh the unit (rewrites ExecStart/ConditionFileIsExecutable to the current path)
sudo /usr/local/bin/surge service install

# Restart and verify
sudo /usr/local/bin/surge service start
surge service status

# Attach the TUI to the running service
surge connect
```

- Run `service install`/`start` through the **installed** path
  (`/usr/local/bin/surge`), never `sudo ./surge` from the repo — otherwise
  `ExecStart` points back at the home directory and SELinux blocks it again.
- `surge service install` refreshes an existing unit in place, so
  `service uninstall` is not required first.
- Stop using a Homebrew-installed binary before switching to a source build, and
  remove any leftover `/usr/local/bin/surge` symlink (`sudo rm -f …`) so the copy
  above replaces it with a real file.
- The daemon uses the service user's home for config/state (`/root/...`) and
  writes runtime files (port/token) to `/run/surge`, so `surge connect`
  auto-detects it without `sudo`.
- If the service still will not stay up, inspect the journal:
  `journalctl -u surge.service -n 50 --no-pager`.

### 4. Remote TUI

`surge` and `surge server` bind the HTTP API to `0.0.0.0` (all interfaces) by default.
This means the server is accessible via `localhost` (127.0.0.1) as well as your local network IP.

The API is token-protected. Generate/read your token by running:

```bash
# Get the standard token
surge token

# Get the token if Surge is installed as a system service
surge service token
```

Alternatively, you can find it in the TUI under **Settings > Extension**.

### 3. Remote TUI

Connect to a running Surge daemon (local or remote).

```bash
# Connect to local server (auto-detected)
surge connect

# Connect to a remote daemon
surge connect 192.168.1.10:1700 --token <token>

# Equivalent global-flag form
surge --host 192.168.1.10:1700 --token <token>
```

By default, `surge connect` uses:

- `http://` for loopback and private IP targets
- `https://` for public/hostname targets

### 4. Global Connection Flags (CLI + TUI)

These global flags are available on all commands:

- `--host <host:port>`: target server for TUI and CLI operations.
- `--token <token>`: bearer token for authentication.

Environment variable fallbacks:

- `SURGE_HOST`
- `SURGE_TOKEN`

### 5. Server Mode with Docker Compose

Download the compose file and start the container:

```bash
wget https://raw.githubusercontent.com/SurgeDM/Surge/refs/heads/main/docker/compose.yml
docker compose up -d
```

Get the API token:

```bash
docker compose exec surge surge token
```

Save this token - you'll need it to authenticate API requests and connect remotely.

Check downloads/API availability:

```bash
docker compose exec surge surge ls
```

View logs:

```bash
docker compose logs -f surge
```

---

## Web UI

Surge ships a browser client for the same daemon the TUI and CLI talk to. Queue,
pause, resume, and remove downloads from a phone, a tablet, or any machine on
your network, with live progress over server-sent events.

The Web UI is a separate component: it is a static Vite/React app that talks to a
`surge server` over HTTP, so it needs a daemon to be useful. Deploy the daemon
first with the [Docker Compose section](#5-server-mode-with-docker-compose) above
(or any other method), then deploy the UI.

### Deploy with Docker

The repository root has a `docker-compose.yaml` that builds and serves the Web UI.
It publishes on port **1777** by default:

```bash
git clone https://github.com/SurgeDM/Surge.git
cd Surge
docker compose up -d --build
```

Open `http://localhost:1777`, then open **Server settings** (the gear icon in the
header) and enter your daemon's URL and token. The token comes from
`surge token`, or from the TUI under **Settings → Extension**; see
[Remote TUI](#4-remote-tui).

To publish on a different port, copy the sample environment file and edit it:

```bash
cp .env.example .env
# SURGE_WEB_PORT=8080
docker compose up -d
```

> [!IMPORTANT]
> The compose file sets the **host** port from `SURGE_WEB_PORT` and leaves the
> container's internal port at 1777. Changing the host port is enough — do not
> also change the container side, or the published port stops reaching nginx.

### Pointing the UI at your daemon

Because the browser makes the request, the URL you enter must be reachable from
the **browser**, not from the container serving the UI. Two consequences:

- A daemon running on the same host as the container is **not** at
  `http://127.0.0.1:1700` — inside a container that address is the container
  itself. Use the host's LAN IP (`http://192.168.1.10:1700`), or
  `http://host.docker.internal:1700` on Docker Desktop and OrbStack.
- To reach the UI from another device, the daemon must be reachable too. Surge
  binds `0.0.0.0` by default, so the LAN IP works as long as your firewall
  allows port 1700.

The connection is stored in that browser's local storage, per device.

### Run it locally for development

```bash
cd web
npm install
npm run dev      # http://localhost:5173
npm run build    # type check plus production build
npm run test     # node:test, no test dependencies
npm run lint     # oxlint
```

The Docker image builds the app with `npm run build` and serves the result from
nginx on an `nginx:alpine-slim` base. See [`web/README.md`](web/README.md) for
the layout, the endpoints the UI calls, and how it talks to the daemon.

### The version stamp

The UI prints its own version in the bottom-right corner of the page, read from
`web/package.json` at build time. `scripts/hooks/pre-commit` raises that version on
every commit and stages the bump, so a deployed build can be named out loud:

```bash
git config core.hooksPath scripts/hooks
```

That is one-time per clone. A relative `core.hooksPath` resolves against `.git`, not
the working tree, so the value has to be relative to the repository root as written.

---

## Fonts

Surge ships a bundled Nerd Font for the TUI, but your terminal controls the
actual font selection. See the [customization guide](docs/guides/customize-surge.md) for install steps and
licensing details.

---

## Benchmarks

We tested Surge against standard tools. Because of our connection optimization logic, Surge significantly outperforms single-connection tools.

| Tool            | Time             | Speed                | Comparison    |
| --------------- | ---------------- | -------------------- | ------------- |
| **Surge** | **28.93s** | **35.40 MB/s** | **-**  |
| aria2c          | 40.04s           | 25.57 MB/s           | 1.38× slower |
| curl            | 57.57s           | 17.79 MB/s           | 1.99× slower |
| wget            | 61.81s           | 16.57 MB/s           | 2.14× slower |

> _Test details: 1GB file, Windows 11, Ryzen 5 5600X, 360 Mbps Network. Results averaged over 5 runs._

We would love to see you benchmark Surge on your system!

To compare fixed and adaptive concurrency under deterministic throttling, run:

```bash
go test ./internal/strategy/concurrent -run '^$' -bench BenchmarkThrottle -benchtime=1x -count=5
```

The native Go benchmark runs persistent-overload and burst-recovery workloads
with both policies and reports elapsed time, request amplification, throttled
requests, and peak accepted concurrency.

---

## Browser Extension

The Surge extension intercepts browser downloads and sends them straight to your terminal. It communicates with the Surge client on port **1700** by default.

> [!IMPORTANT]
> An **Auth Token** is required to connect the extension to your Surge server. This can be obtained from the TUI under **Settings > Extension**, or by running `surge token` (or `surge service token` if installed as a system service).

### Chrome / Edge / Brave

1. **Stable:** [Get the Extension from Chrome Web Store](https://chromewebstore.google.com/detail/surgedm/cakjmkhlofkhjmfkjlclgbfdklhdnkgl)
2. **Development:**
   - Download `extension-chrome.zip` from the latest GitHub release.
   - Unzip it somewhere on disk.
   - Open your browser and navigate to `chrome://extensions`.
   - Enable **"Developer mode"** in the top right corner.
   - Click **"Load unpacked"**.
   - Select the unzipped `extension-chrome` folder.
   - Click the Surge icon in your browser toolbar and enter your **Auth Token** in the settings.

### Firefox

1. **Stable:** [Get the Add-on](https://addons.mozilla.org/en-US/firefox/addon/surge/)
2. **Development:**
   - Download `extension-firefox.zip` from the latest GitHub release.
   - Navigate to `about:debugging#/runtime/this-firefox`.
   - Click **"Load Temporary Add-on..."**.
   - Select the zip file (or unzip and select `manifest.json`).
   - Click the Surge icon in your browser toolbar and enter your **Auth Token** in the settings.

---

## Acknowledgements

Huge thanks to the teams and sponsors helping us build and ship Surge:

- [FOSS United](https://fossunited.org/) for their generous grant supporting our development. We genuinely love and appreciate everything they do to build and nurture the FOSS ecosystem in India.
- [Charm](https://charm.sh/) for the incredible terminal UI ecosystem (Bubble Tea, Lip Gloss, and more).
- [GoReleaser Pro](https://goreleaser.com/pro/) for release automation (provided free for open source).

---

## Community & Contributing

We love community contributions! Whether it's a bug fix, a new feature, or just cleaning up typos.
PRs are always welcome. For a quick guide, see [CONTRIBUTING.md](CONTRIBUTING.md).

You can check out the [Discussions](https://github.com/SurgeDM/Surge/discussions) for any questions or ideas, or follow us on [X (Twitter)](https://x.com/SurgeDownloader)!

## License

Distributed under the MIT License. See [LICENSE](https://github.com/SurgeDM/Surge/blob/main/LICENSE) for more information.

---

<div align="center">
<a href="https://star-history.dera.page/#SurgeDM/Surge&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://star-history.dera.page/svg?repos=SurgeDM/Surge&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://star-history.dera.page/svg?repos=SurgeDM/Surge&type=Date" />
   <img alt="Star History Chart" src="https://star-history.dera.page/svg?repos=SurgeDM/Surge&type=Date" />
 </picture>
</a>

<br />
If Surge saved you some time, consider giving it a ⭐ to help others find it!
</div>
