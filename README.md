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

