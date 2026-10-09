# Penguins Chef Documentation 👨‍🍳🐧

`penguins-chef` is the provisioning engine designed for `penguins-eggs`. It applies modular, deterministic, and verifiable recipes to forge clean Linux systems and custom respin environments.

---

## ⚙️ Core Architecture & Contract

The engine executes autonomous and complete recipes through a rigorous, verifiable pipeline. Unlike traditional configuration managers, it relies on real system states rather than unverified checkpoints.

### The Execution Contract
1. **Read & Select:** Parse a strict YAML recipe schema and select a target profile based on the host/family distribution.
2. **Validate Profile:** Validate all definitions before performing any system modifications.
3. **Configure Repositories:** Add declared repository files (e.g., APT `.list` or `.sources`), then prepare the package manager.
4. **Pre-check Packages:** Check the availability of all required packages *before* starting any installation.
5. **Atomic Transaction:** Inspect installed packages, install missing ones in a single transaction, and verify the outcome for each package.
6. **Deploy Configuration Files:** Compare, write, and verify configuration files (UTF-8 YAML, default permissions `0644`). Symbolic link destinations are strictly rejected.
7. **Manage Hostname:** Check, set, and verify the system hostname (for costumes or explicit declarations), atomically updating `/etc/hosts` for local loopback resolution.
8. **Manage Services:** Check, enable, and verify services through the init backend (services are enabled, not immediately started).

*Note on Atomicity:* "Atomic" here means a unit with a verifiable result rather than a full system rollback transaction. Files are replaced via temporary file renaming in the same directory. Every execution re-reads the real state of packages, configurations, and services.

---

## 📦 Recipe Composition & Tree Structure

The historical dichotomy between "costumes" (full desktops) and "accessories" (single functions) is unified: every element is a standalone recipe. Complexity is handled through explicit composition using the `include` directive.

### Tree Layout
```text
recipes/
├── base/                   # Base system configurations and core utilities
├── de/                     # Upstream/vanilla Desktop Environments (plasma, xfce4, etc.)
├── dm/                     # Display Managers (lightdm, gdm, sddm)
├── development/            # Development tools and IDEs (golang, vscode, devel)
├── graphics/               # Graphic applications (gimp)
├── multimedia/             # Multimedia applications (vlc)
├── office/                 # Productivity suites (libreoffice)
├── education/              # Educational software
├── game/                   # Games and entertainment
├── network/                # Browsers and network tools
├── settings/               # Configuration tools
├── system/                 # System utilities
├── utility/                # Daily utilities and accessories
└── costumes/               # Complete tailored configurations (base + dm + de + branding + sysroot)
    ├── colibri/
    │   ├── colibri.yaml
    │   └── sysroot/
    └── quirinux/
        ├── quirinux.yaml
        └── sysroot/
```

### Explicit Composition (`include`)
To respect the DRY (Don't Repeat Yourself) principle, recipes can include other modules. For example, a complete costume like `colibri.yaml` aggregates base components, a display manager, a desktop environment, and local assets:

```yaml
version: 1
name: colibri-desktop
include:
  - ../../base/base.yaml
  - ../../dm/lightdm.yaml
  - ../../de/xfce4.yaml
sysroot: colibri/sysroot
profiles:
  debian:
    packages:
      - xfce4-whiskermenu-plugin
      - xfce4-pulseaudio-plugin
  archlinux:
    packages:
      - xfce4-whiskermenu-plugin
      - xfce4-pulseaudio-plugin
  fedora:
    packages:
      - xfce4-whiskermenu-plugin
      - xfce4-pulseaudio-plugin
  opensuse:
    packages:
      - xfce4-whiskermenu-plugin
      - xfce4-pulseaudio-plugin
      - pipewire
      - pipewire-pulseaudio
      - wireplumber
```

---

## 🚀 Supported Families & Backends

`penguins-chef` decouples package managers and init systems across major Linux distributions:

* **Arch Linux / Manjaro:** Uses `pacman`. Prepares by running `pacman -Syu --noconfirm` (system update is mandatory to prevent partial upgrades).
* **Debian / Ubuntu / Devuan:** Uses `apt`. Prepares by running `apt-get update --error-on=any`.
* **Fedora:** Uses `DNF` (including DNF5). Refreshes via `dnf --refresh makecache`, checks availability with `dnf repoquery --available`, and installs via `dnf install -y` without performing a full system upgrade.
* **openSUSE Tumbleweed:** Uses `zypper --non-interactive refresh` with exact XML repository search. Uses RPM for installed package verification.

---

## 🛠️ Usage & Dry-Run Verification

Before making any changes to the system, always inspect the fully resolved execution plan using `--dry-run`:

```bash
# Simulate LightDM provisioning across different families
go run . apply examples/provision/lightdm.yaml --dry-run --family debian --init systemd
go run . apply examples/provision/lightdm.yaml --dry-run --family archlinux --init systemd

# Simulate Colibri desktop provisioning on Arch Linux
go run . apply examples/provision/colibri.yaml --dry-run --family archlinux --init systemd
```

### Applying a Recipe on a Real Host
On a target machine where family and systemd are automatically detected by the host:

```bash
# Apply a specific display manager
sudo chef apply recipes/dm/lightdm.yaml

# Apply a complete custom costume
sudo chef apply recipes/costumes/colibri/colibri.yaml
```

Re-running the apply command acts as an idempotent check: packages, files, and services are verified against the real system state without redundant operations.