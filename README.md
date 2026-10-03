# penguins-tailor

**penguins-tailor** is a standalone, lightweight tool written in Go to apply explicit system configuration recipes to Linux distributions. Recipes and their assets live alongside the project or wherever the user keeps them; Tailor does not download or depend on a separate costume repository.

---

## 🚀 Features

- **Apply**: Validate and execute an operation-based recipe (`tailor apply <recipe.yaml>`), with a reviewable dry run.
- **Export**: Transfer native packages (`tailor export pkg`) or execution logs and reports (`tailor export log`) to remote storage via SSH.
- **Build**: Integrated packaging tool to compile binaries and produce native distribution packages (`tailor tools build`).
- **Repo**: Configure or remove official `penguins-eggs.net` repositories (`sudo tailor tools repo [add|rm]`).
- **Distro-Aware**: Automatically identifies target distributions (Debian, Ubuntu, Arch, Alpine, Fedora, openSUSE, etc.) and generates assistance prompts if non-Debian package managers are present.

The operation-based engine supports explicit profiles for Debian, Arch Linux, Fedora and openSUSE. See [the execution model and examples](docs/atomic-execution.md) for its current scope and limitations.

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

`apply` builds an explicit repository → packages → configuration → init plan.
Use `--dry-run` to inspect the operations before applying them.

```bash
tailor apply examples/provision/lightdm.yaml --dry-run --family archlinux --init systemd
```

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
