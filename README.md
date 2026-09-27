# penguins-tailor

**penguins-tailor** is a standalone, lightweight tool written in Go to manage and apply system configurations, desktop environments, and themes ("costumes") to Linux distributions.

`tailor` works in conjunction with costume repositories ("wardrobes") containing declarative YAML definitions, package lists, and system configuration files. By default, it connects to the official [penguins-wardrobe](https://github.com/pieroproietti/penguins-wardrobe) repository, but it can also work with any third-party or custom wardrobe by supplying the Git repository URL.

---

## 🚀 Features

- **Get**: Download or update a costume repository (`tailor get`).
- **List**: Enumerate available costumes and their descriptions (`tailor list`).
- **Show**: Inspect detailed information and packages required by a costume (`tailor show <costume>`).
- **Wear**: Seamlessly apply a costume to the system (`sudo tailor wear <costume>`), configuring repositories, packages, sysroot configurations, and user skel settings.
- **Export**: Transfer native packages (`tailor export pkg`) or execution logs and reports (`tailor export log`) to remote storage via SSH.
- **Build**: Integrated packaging tool to compile binaries and produce native distribution packages (`tailor tools build`).
- **Repo**: Configure or remove official `penguins-eggs.net` repositories (`sudo tailor tools repo [add|rm]`).
- **Distro-Aware**: Automatically identifies target distributions (Debian, Ubuntu, Arch, Alpine, Fedora, openSUSE, etc.) and generates assistance prompts if non-Debian package managers are present.

> NOTE: At present, Tailor is only tested on the Debian family of distributions (Debian, Devuan, Ubuntu, and their derivatives). We have plans to expand support to Arch Linux and possibly other distributions in the future. The major hurdle is the inconsistency in package naming conventions, which we might eventually address through AI.

---

## 📦 Installation

```bash
git clone https://github.com/pieroproietti/penguins-tailor.git
cd penguins-tailor
make
sudo make install
```

---

## 👔 Command Reference

### Basic Commands

- **`tailor get [url]`**
  Clones or updates the costumes repository into `~/.wardrobe`. If no URL is specified, it defaults to the official repository (`https://github.com/pieroproietti/penguins-wardrobe`). You can also specify an alternative or third-party wardrobe repository and an optional branch (`-b, --branch`):
  ```bash
  # Official penguins-wardrobe repository (default)
  tailor get

  # Custom or third-party wardrobe repository
  tailor get https://github.com/charliemartinez/penguins-wardrobe

  # Custom wardrobe repository specifying a branch
  tailor get https://github.com/charliemartinez/penguins-wardrobe -b develop
  ```

  **Flags:**
  - `-u, --url <url>`: URL of the costumes repository.
  - `-b, --branch <branch>`: Branch of the costumes repository.

- **`tailor list`**
  Lists all available costumes found in the repository along with a brief description.
  ```bash
  tailor list
  ```

- **`tailor show <costume>`**
  Shows detailed metadata for a specific costume (e.g. description, supported distributions, packages, accessories, and commands).
  ```bash
  tailor show colibri
  ```

- **`tailor wear <costume>`**
  Applies the specified costume to the system. Requires root privileges (`sudo`). You can also specify an optional branch (`-b, --branch`) to automatically switch or clone the costumes repository on that branch before applying:
  ```bash
  sudo tailor wear colibri

  # Simulate costume application without modifying the system (does not require root)
  tailor wear colibri --dry-run
  ```
  **Flags:**
  - `-b, --branch <branch>`: Branch of the costumes repository.
  - `-n, --dry-run`: Simulate costume installation without making changes (allows running without root).
  - `--linear`: Use linear standard output without split screen TUI.

---

### Export Commands

The **`export`** command suite automates the transfer of generated artifacts and logs to configured remote destinations:

#### 1. Export Native Packages (`tailor export pkg`)
Transfers compiled native packages (`.deb`, `.rpm`, `.pkg.tar.zst`, `.apk`) corresponding to the current distribution family to the remote storage server (`root@192.168.1.2:/eggs/`). It establishes an SSH multiplexed connection for efficient multi-file transfer.

```bash
# Export the built package
tailor export pkg

# Clean old versions on the remote server before exporting
tailor export pkg --clean
```

**Flags:**
- `--clean`: Removes previous versions of the package matching the distribution pattern on the remote server before uploading the new one.

#### 2. Export Logs and Reports (`tailor export log`)
Collects and uploads the main tailor log file (`/var/log/tailor.log`) and the latest detailed wear report (`/var/log/tailor/tailor-report-*.txt`) to the target server in a single SSH session without requiring manual file copying.

```bash
# Export logs to default remote destination
tailor export log

# Export logs with custom SSH user, IP, and destination directory
tailor export log -u artisan -i 192.168.1.50 -d /home/artisan/logs
```

**Flags:**
- `-u, --user <username>`: Remote SSH username (default: `artisan`).
- `-i, --ip <address>`: Remote IP address or hostname (default: `192.168.1.2`).
- `-d, --dir <path>`: Destination directory on the remote machine (default: `/home/artisan`).

---

### Packaging & Auxiliary Tools

- **`tailor tools build`**
  Compiles binaries and generates distribution-specific packages (`.deb` for Debian/Ubuntu, `PKGBUILD`/`.pkg.tar.zst` for Arch Linux, `.rpm` for Fedora/openSUSE, `.apk` for Alpine). Must be run as a regular user (not root).
  ```bash
  tailor tools build
  ```

- **`tailor tools repo [add|rm]`**
  Configures or removes the official `penguins-eggs.net` repositories and GPG keys for the host system's package manager (APT, Pacman, DNF, Zypper, APK). Requires root privileges (`sudo`).
  ```bash
  # Add official repository and GPG keys
  sudo tailor tools repo add

  # Remove repository configuration and keys
  sudo tailor tools repo rm
  ```

---

## 🙏 Acknowledgements

Special thanks to **[Charlie Martínez](https://github.com/charliemartinez)** [Quirinux](https://quirinux.org) for his invaluable support, extensive testing, ideas, and close collaboration during the development and experimentation of `penguins-tailor`.

---

## 📜 License

MIT License. Copyright (c) 2026 Piero Proietti.


## Experimental desktop selection (`devel`)

Costumes can opt into desktop configuration with these top-level fields:

```yaml
desktop: xfce
display_manager: lightdm
session_type: x11
init: auto
```

The first implementation supports **LightDM with systemd, SysVinit (update-rc.d), or OpenRC**.
Desktop identifiers are `gnome`, `plasma`, `xfce`, `cinnamon`, `mate`, `lxqt`,
and `budgie`. SDDM, GDM and COSMIC are not yet implemented. Existing costumes without these fields keep their previous behavior.
Put the fields in the costume itself; nested accessories do not select the login.

On Debian-family systems, a minimal recipe without package lists or accessories
gets the desktop package, `lightdm` and `lightdm-gtk-greeter`. Existing recipes
with package lists or accessories retain their curated package selection. On other families,
the desktop, LightDM and a working greeter must already be installed; no package
installation or arbitrary costume scripts are added to the configuration-only path.

Tailor checks installed session files and the LightDM service after installing
accessories and before copying the costume sysroot. Accessory overlays retain
their existing ordering. On non-Debian systems the check precedes all overlays.
It configures the default session and enables LightDM after sysroot (and, on
Debian, finalization). It does not restart the login service or
change the default boot target. Package installation scripts may independently
manage services. An existing graphical boot target is assumed.

`init` accepts `auto` (also the default), `systemd`, `sysvinit`, or `openrc`.
Automatic detection uses runtime markers and PID 1; it does not assume systemd
just because `systemctl` is installed. An explicit value must match the running
system. This selects the service backend; it never installs or replaces init.
Chroots/offline roots are not supported. SysVinit uses `update-rc.d` and disables
other installed login-manager scripts. OpenRC requires `/etc/init.d/lightdm`,
enables it in `default`, and removes known competing login entries from that
runlevel. Custom OpenRC runlevels are not managed. These operations do not start,
stop or restart the active login service. Legacy recipe scripts may do more.

`session_type` accepts `x11`, `wayland`, or `auto` (also the default when omitted).
An explicit protocol never falls back to another one. In this prototype, `auto`
selects only an unambiguous installed session; otherwise it reports an error and
asks for an explicit protocol. Identical session names present in both X11 and
Wayland directories are rejected because LightDM's basename setting cannot
express that distinction. The login screen's own display protocol is unchanged.

The default is written to `[Seat:*] user-session` in `/etc/lightdm/lightdm.conf`,
preserving other settings. Existing per-user session choices and more specific
seat settings may override it. Desktop-specific configuration remains in sysroot.
This is a prototype: graphics-driver compatibility, greeter readiness, customized
session paths, and every distribution/version combination are not validated.
Configuration writes are not transactional if service activation fails.

An example local wardrobe is in `examples/desktop`. To inspect the simulated flow:

```bash
go build -o /tmp/tailor-devel .
cd examples/desktop
/tmp/tailor-devel wear xfce-lightdm --dry-run --linear
```

Dry-run prints the desktop plan and skips package refresh, installation and login
configuration; installed-session/service validation is deferred to a real run.
The existing reporting/logging mechanism can still write diagnostic files.
Try a real wear in a disposable VM before using this experimental branch on a
workstation. No real wear is required to run `go test ./...`.

Configuration reference: [LightDM's configuration](https://github.com/ubuntu/lightdm/blob/main/data/lightdm.conf).

Service references: [Debian update-rc.d](https://manpages.debian.org/unstable/init-system-helpers/update-rc.d.8.en.html), [OpenRC guide](https://github.com/OpenRC/openrc/blob/master/user-guide.md).

### Wardrobe v3 on Tailor devel

This development branch selects the `v3/` collection when present, then
falls back to `v2/` or an unversioned wardrobe. The same selection applies to
`list`, `show`, and `wear`, including local development wardrobes when
`~/.wardrobe` is absent. A selected collection supplies its own costumes,
accessories, scripts, and branding; missing costumes are not mixed across versions.

`tailor get` clones or updates the entire repository, so it also downloads
`v3/` once that directory is published on the selected remote branch.
It does not automatically switch to the wardrobe's `devel` branch.
If the new collection is published there, use `tailor get --branch devel`.
Local, uncommitted wardrobe changes cannot be downloaded with `get`.
