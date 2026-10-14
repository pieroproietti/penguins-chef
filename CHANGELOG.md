# Changelog

## Release Notes: penguins-chef v26.10.14 - 2026-10-14
This release introduces full support for OpenKylin Linux, implements an interactive dual-panel TUI interface for `chef apply`, adds robust non-systemd and Devuan support with automatic init detection, adds AI assistant context (`llms.txt`), and reorganizes recipes into modular `dm` and `de` structures.

### 🐧 OpenKylin Distribution Support
* **Distribution Identification**: Added OpenKylin (`ID=openkylin`, codename `huanghe`) detection in `pkg/distro`, properly mapped to the Debian family.
* **Modern DEB822 Sources**: Added recognition for OpenKylin DEB822 repository configuration (`/etc/apt/sources.list.d/openkylin.sources`).
* **Cross-Family Simulation**: Enabled `--family openkylin` support in `chef apply --dry-run`, automatically defaulting init to `systemd`.
* **Packaging & Scripting**: Updated build helper script `m` to support installing `.deb` packages on OpenKylin.

### 🖥️ Dual-Panel TUI for `chef apply`
* **Interactive Terminal Interface**: Introduced a two-panel TUI (powered by Bubble Tea) when running `chef apply` interactively without arguments.
* **Browsing & Inspection**: Left panel provides hierarchical category navigation across costumes and recipes, while the right panel shows detailed recipe specifications, profiles, packages, and services.
* **Streamlined Provisioning**: Visual confirmation and selection before cooking/applying recipes directly from the terminal console.

### ⚙️ Non-Systemd & Devuan Compatibility
* **Decoupled Service Management**: Service handling for APT distributions is now decoupled from strict systemd requirements, enabling native SysVinit support.
* **Devuan Distribution Support**: Added full support for Devuan GNU+Linux, automatically detecting runtime init and defaulting to `sysvinit`.
* **Conditional Package Filtering**: Systemd-specific packages (such as `systemd-timesyncd` or `dbus-user-session`) are dynamically skipped on non-systemd systems.
* **Fluid Cross-Family Dry-Run**: Seamless `--dry-run` simulation across distinct distribution families (`archlinux`, `debian`, `devuan`, `fedora`, `openkylin`, `opensuse`) and init systems (`systemd`, `sysvinit`) without requiring manual `--init` flags.

### 🍳 Recipe Architecture & Multimedia
* **DM and DE Restructuring**: Organized desktop recipes into clean modular categories: `dm/` (display managers: GDM, LightDM, SDDM) and `de/` (desktop environments: GNOME, KDE Plasma, XFCE, Cinnamon, MATE, LXQt).
* **Music Pro Recipe**: Added dedicated `music-pro` audio/multimedia recipe with comprehensive packaging definitions.
* **Directory Retention**: Added `.keepit` files to ensure empty recipe skeleton directories are tracked reliably in git.

### 🤖 AI Agent Documentation & Context
* **LLM Context (`llms.txt`)**: Added `llms.txt` and symlink `llm.txt` providing complete CLI commands, recipe formats, and architecture guidelines for AI coding assistants.
* **Documentation Refresh**: Updated documentation with automatic init detection details and cross-family simulation workflows.

## Release Notes: penguins-chef v26.10.7 - 2026-10-07
This release marks the official transition and rebranding from penguins-tailor to penguins-chef, introducing the `chef` CLI command suite and reintroducing `chef get` for recipe repository management.

### 🐧 Project Rebrand to penguins-chef
* **New CLI Binary**: Rebranded tool to `chef` (`penguins-chef`), with commands `apply`, `get`, `tools`, and `version`.
* **Workspace Directory**: Standardized recipe directory on `~/.chef`.
* **Recipe System**: Introduced declarative YAML recipes and costumes for desktop environments, display managers, development environments, and office applications.

### 📥 Chef Get Command
* **Repository Synchronization**: Added `chef get` command to clone or update recipes and costumes from remote Git repositories directly into `~/.chef`.

## Release Notes: penguins-tailor v26.9.4 - 2026-09-04
This release adds costume-provided branding installation, makes technical logs and wear reports distribution-specific, and ensures the installed branding tree remains accessible with consistent root permissions.

### 🎨 Costume Branding
* **Declarative Branding Selection**: Costumes can now select a branding bundle through the `branding` field in `index.yaml`.
* **Atomic Branding Installation**: The selected bundle is validated, copied into `/etc/penguins-eggs.d/branding`, and replaces any previously active branding without retaining the source bundle directory.
* **Safe Cleanup and Dry Runs**: An empty selection removes stale branding, while dry-run mode reports the intended operation without changing the target.
* **Consistent Permissions**: The active branding root is normalized to `0755`, preventing restrictive temporary-directory permissions from making installed assets inaccessible.

### 🧰 Distribution-Specific Diagnostics
* **Identifiable Technical Logs**: Wear logs include the detected distribution identity in their filenames, keeping diagnostics distinct across systems.
* **Identifiable Wear Reports**: Generated reports likewise include the distribution identity while preserving the legacy filename format when no identity is available.
* **Expanded Regression Coverage**: Added tests for distribution identity detection, log and report naming, branding validation, replacement, cleanup, dry-run behavior, and permissions.

## Release Notes: penguins-tailor v26.9.2 - 2026-09-02
This release strengthens the `tailor wear` package-management lifecycle, replaces the split-screen dashboard with clearer progressive terminal output, centralizes technical logging and detailed reports, and makes package results and repository-dependent kernel header handling more reliable.

### 📦 Package Management & Distribution Safety
* **Package Manager Abstraction**: Introduced a dedicated `PackageManager` interface and moved APT-specific refresh, upgrade, installation, healing, and installed-package checks into a separate implementation (`pkg/tailor/package-manager.go`, `pkg/tailor/package-manager-apt.go`).
* **Distribution Validation**: Added explicit distribution-family checks before applying a costume and removed the obsolete AI prompt path from the wear pipeline.
* **Portable Repository Upgrades**: Repository setup now delegates refresh and upgrade operations to the active package manager instead of invoking APT-specific upgrade logic directly.
* **Early Metadata Refresh**: Package metadata is refreshed before wardrobe and costume setup, and a failed initial refresh now stops the operation before any package or kernel-header work begins.

### 🔄 Repository & Kernel Header Ordering
* **Repository-Aware DKMS Preparation**: Moved the running-kernel header check after costume repository configuration and metadata updates, while keeping it before package installation so newly configured sources can provide the required header packages.
* **Single Main-Costume Check**: Kernel headers are verified once for the main costume rather than repeatedly for each accessory.
* **Lifecycle Test Coverage**: Added tests covering metadata refresh failure, repository refresh, kernel-header verification, and package-installation ordering.

### 🖥️ Progressive Wear Output
* **Progressive Terminal Renderer**: Replaced the fixed split-screen dashboard with a progressive action stream that preserves completed steps and keeps the current action visibly updated.
* **Clearer Costume Progress**: Added the main costume to the completed-step history, improved package counters, and removed the redundant top header divider.
* **Report Review Flow**: The complete wear output remains visible before the interactive reset prompt and final summary, making warnings and command results easier to review.

### 📝 Logging, Reports & Package Outcomes
* **Central Technical Log**: Added a concurrency-safe structured logger and raw command-output capture under `/var/log/tailor/tailor.log`, consolidating operational diagnostics in one location (`pkg/utils/technical_logger.go`, `pkg/utils/exec.go`).
* **Unavailable vs. Failed Packages**: Package results now distinguish packages absent from configured repositories from packages that were available but failed during installation.
* **Detailed Wear Reports**: Timestamped reports under `/var/log/tailor/` now include separate Installed, Unavailable, and Could NOT be installed sections, with the final summary reporting the relevant totals and report path.
* **Expanded Regression Tests**: Added coverage for package-manager behavior, technical logging, report contents, progressive output, and package-result aggregation.

## Release Notes: penguins-tailor v26.8.31 - 2026-08-31
This release introduces the `tailor tools repo` command to manage official penguins-eggs repositories across diverse Linux distributions, refactors the execution lifecycle to defer costume finalization after accessory processing, introduces explicit distinction between sequence and finalization commands, adds an interactive terminal pause before displaying summary reports, and enables automatic reboot execution when required by a costume.

### 🐧 Official Repository Management (`tailor tools repo`)
* **Multi-Distro Repository Management**: Added `tailor tools repo [add|rm]` (`cmd/tools_repo.go`, `pkg/repo/`) allowing users to easily configure or remove official `penguins-eggs.net` repositories and GPG signing keys on Debian/Ubuntu (DEB822 and classic list formats), Arch Linux/Manjaro (`pacman.conf` / `pacman-key`), Fedora/EL9 (`dnf`/`rpm`), openSUSE Leap (`zypper`), and Alpine Linux (`apk`).
* **Documentation & Reference Updates**: Documented repository management features and command usage examples in `README.md`.

### 🔄 Execution Flow & Sequence/Finalization Separation
* **Deferred Finalization**: Reordered the costume execution pipeline (`pkg/tailor/wear.go`) so that costume sysroot configuration sync and finalization scripts run strictly after all accessories have been installed and configured.
* **Sequence vs. Finalize Commands**: Added support for intermediate sequence commands (`sequence.cmds`) alongside finalization commands (`finalize.cmds`), while maintaining backward compatibility with legacy `cmds` syntax (`pkg/tailor/types.go`, `pkg/tailor/wear.go`).
* **Unit Test Coverage**: Added dedicated unit tests (`pkg/tailor/wear_test.go`) validating sequence and finalization command parsing and execution order.

### 🖥️ Interactive UX & Automated Reboot Handling
* **Interactive Terminal Review Pause**: Added an interactive prompt (`Press Enter to continue to summary report...`) at the conclusion of costume execution, allowing users to review logs and command outputs in the split-screen TUI before rendering the final summary box.
* **Automatic System Reboot**: When a costume specifies `reboot: true`, `tailor wear` now displays a reboot countdown notification and automatically triggers a system restart upon completion.

## Release Notes: penguins-tailor v26.8.30 - 2026-08-30
This release introduces real-time target name feedback during costume and accessory execution, refines terminology to "system configuration" (sysroot), standardizes all user-facing console output and internal logs into English (i18n), optimizes the package management pipeline by removing redundant batch chunking, polishes the split-screen TUI header to a concise 3-line layout, expands automated CI testing across multi-architecture distributions ("Hammers" workflow), and introduces a comprehensive unit testing suite.

### 🎯 Real-Time Execution Feedback & Terminology Refinement
* **Active Target Name in Progress Messages**: Updated split-screen action messages (`pkg/tailor/wear.go`) to display the active costume or accessory name (e.g., `"<target>, installing packages..."`, `"<target>, running finalization scripts..."`), providing granular visibility during multi-stage runs.
* **System Configuration Terminology**: Renamed "sysroot overlay" to "system configuration (sysroot)" across CLI actions, log messages, and documentation (`README.md`, `pkg/tailor/wear.go`) to more accurately reflect filesystem configuration syncing.
* **Accessory Step Log Isolation**: Prevented subsidiary accessory installation steps from polluting the primary costume's high-level step log in split-screen mode.

### 🌐 Full English Internationalization (i18n) & CLI Polish
* **Complete English Translation**: Translated all remaining Italian console output, terminal prints, summary messages, warnings, execution logs, and code comments to English across `cmd/wear.go`, `pkg/tailor/report.go`, `pkg/tailor/repositories.go`, `pkg/tailor/tailor.go`, `pkg/tailor/types.go`, `pkg/tailor/wear-logic.go`, `pkg/tailor/wear.go`, and `pkg/utils/`.
* **Atelier Repository Terminology**: Updated `tailor get` help and flag descriptions (`cmd/get.go`) to clarify repository terminology and wardrobe destination (`~/.wardrobe`).
* **TUI Header & Title Layout**: Cleaned up the split-screen TUI appearance (`pkg/utils/split_screen.go`, `pkg/tailor/wear.go`) by standardizing on a clean 3-line header and removing decorative icons for a polished CLI experience.

### ⚡ Package Pipeline Optimization & Dry-Run Enhancements
* **Direct Package Installation**: Removed package batching and chunking routines (`processPackagesInBatches` in `pkg/tailor/wear-logic.go`), simplifying the package installation pipeline and reducing execution overhead.
* **Dry-Run Simulation Parity**: Enhanced `--dry-run` simulation mode to accurately simulate interactive package configurations, system configuration overlays, and finalization scripts without requiring root permissions.

### 🔨 Multi-Arch CI Matrix ("Hammers") & Test Coverage
* **Multi-Distribution Packaging CI**: Added GitHub Actions workflow (`.github/workflows/hammers.yml`) to compile and package native distributions across Debian (amd64, arm64, riscv64), Ubuntu, Fedora, openSUSE, Alpine, Arch Linux, and Manjaro Linux.
* **Automated Unit Test Suites**: Added unit testing coverage in `pkg/tailor/wear_test.go`, `cmd/wear_test.go`, and `pkg/utils/split_screen_test.go` to validate costume dry-runs, accessory modes, TUI header rendering, and report generation.

## Release Notes: penguins-tailor v26.8.28 - 2026-08-28
This release introduces automatic discovery of `packages.yaml` configurations across costume directories, refactors the costume loading pipeline, removes legacy reconcile logic, and refines wardrobe repository synchronization.

### 📦 Dynamic Package Discovery & Architecture Refactoring
* **Automatic packages.yaml Discovery**: Added recursive discovery for `packages.yaml` across costume subdirectories, automatically aggregating distribution-specific package definitions.
* **Legacy Reconcile Removal**: Removed outdated `reconcile.go` logic to streamline costume staging and execution.
* **Wardrobe Integration**: Improved repository and keyrings configuration handling when applying costumes from wardrobe ateliers.
