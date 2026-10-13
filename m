#!/bin/sh
set -e
make clean package "$@"

. /etc/os-release

case "$ID" in
    alpine)
        sudo apk add --allow-untrusted penguins-chef-*.apk
        ;;

    arch)
        sudo pacman -U --noconfirm penguins-chef-*.pkg.tar.zst
        ;;

    debian|openkylin)
        sudo dpkg -i penguins-chef_*.deb
        ;;
    fedora)
        sudo dnf reinstall -y penguins-chef-*.rpm
        ;;
    opensuse*)
        sudo zypper --no-gpg-checks install -y penguins-chef-*.rpm
        ;;
    *)
        # fallback su LIKE_ID
        case "$ID_LIKE" in
            *arch*)   sudo pacman -U --noconfirm penguins-chef-*.pkg.tar.zst ;;
            *debian*|*devuan*|*ubuntu*|*openkylin*) sudo dpkg -i penguins-chef_*.deb ;;
            *fedora*|*rhel*) sudo dnf install -y penguins-chef-*.rpm ;;
            *) echo "Distro non supportata: $ID"; exit 1 ;;
        esac
        ;;
esac


