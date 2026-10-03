# Colibri public sysroot

`../colibri.yaml` applies this sysroot automatically on every supported family.
Paths beneath `sysroot/` are installed beneath `/`; `etc/skel/` provides the
defaults for new users. Existing home directories are not changed.

This is a reviewed copy of the Colibri desktop assets from Wardrobe v2:
Xfce preferences, standard shell defaults, uinput configuration and wallpaper.
The original wallpaper attribution is retained in
`sysroot/usr/share/backgrounds/colibri/credits.md`.

Do not populate this directory by copying a live home directory. Firefox
profiles, cookies, saved logins, browser sessions, histories, caches, SSH/GPG
keys, keyrings, device-specific display settings and recently used applications
are excluded. Wallpaper EXIF metadata is removed without changing image data.
The source Wardrobe checkout is left intact.

The sysroot contains preferences for fonts, icon and cursor themes; those
optional theme/font packages are not all installed by the desktop recipe.
