# Penguins Chef 👨‍🍳🐧

> The lightweight, modular culinary companion for `penguins-eggs` to forge custom Linux systems.

## 🎯 Project Goals

`penguins-chef` is built with a precise goal: **to provide respin and derivative creators with a clean, modular, and deterministic starting point**, avoiding the tedious and imperfect "post-installation cleanup" work typical of all-in-one live ISOs aimed at end-users.

Instead of starting from a heavy system bloated with unwanted software, `penguins-chef` relies on three core pillars:
1. **Naked Starting Point:** Begin with essential, command-line-only base ISO images (`naked`).
2. **Total Modularity:** Using recipes, install only what you need (Display Manager, Desktop Environment, development tools, multimedia packages).
3. **Tailored Customization (Costumes):** Configure wallpapers, themes, user settings, and system overlays (`sysroot`) in a clean and repeatable way.

*Note:* This tool is designed for developers, maintainers, and power users who want to assemble their operating system "from the kitchen," and is not targeted at end-users looking for a ready-to-use "out of the box" live ISO.

---

## 📂 Recipes Structure (`recipes/`)

The directory structure strictly follows **FreeDesktop** specifications for application categories, paired with standard definitions for graphical environments:

```text
recipes/
├── base/                   # Base system configuration
├── de/                     # Desktop Environments (plasma, xfce4, cinnamon, gnome, etc.)
├── dm/                     # Display Managers (sddm, lightdm, gdm)
├── costumes/               # Visual themes and custom user profiles (albatros, colibri, etc.)
├── development/            # Development tools and IDEs
├── education/              # Educational software
├── game/                   # Games and entertainment
├── graphics/               # Graphics and photo editing tools
├── multimedia/             # Audio and video (e.g., vlc)
├── network/                # Browsers and network tools
├── office/                 # Productivity suites
├── settings/               # Configuration tools
├── system/                 # System utilities
└── utility/                # Accessories and daily utilities
```

---

## 🚀 Quick Start Guide

### 1. Fetch the recipes
Before applying any configuration, download or update the recipe set:
```bash
chef get
```

### 2. Install a Display Manager and Desktop Environment
You can compose your graphical interface by first applying the login manager recipe and then your preferred desktop environment (for example on Arch Linux):
```bash
sudo chef apply recipes/dm/sddm.yaml
sudo chef apply recipes/de/gnome.yaml
```

### 3. Apply a Costume (Customization)
Costumes define visual identity and user configuration files (wallpapers, dotfiles, etc.) via a mirrored `sysroot` structure:
```bash
sudo chef apply recipes/costumes/colibri/colibri.yaml
```

---

## 🛠️ Contributing
Want to add a new recipe or create a new costume?
1. Create the folder or YAML file in the correct category following FreeDesktop standards.
2. If the costume requires configuration files or wallpapers, place them in the relative `sysroot/` subfolder respecting system paths (e.g., `/etc/` or `/usr/share/backgrounds/`).
3. Test the recipe in the field and submit a pull request!
