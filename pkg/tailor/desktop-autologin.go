package tailor

import (
	"fmt"
	"os"
	"os/user"
	"strings"
)

// Use an identified invoking user: never guess which account should get
// passwordless login when Tailor is invoked directly as root.
func desktopAutologinUser() (string, error) {
	name := os.Getenv("SUDO_USER")
	if name == "" {
		name = os.Getenv("DOAS_USER")
	}
	if name == "" {
		current, err := user.Current()
		if err != nil {
			return "", err
		}
		name = current.Username
	}
	return validateAutologinUser(name, user.Lookup)
}

func validateAutologinUser(name string, lookup func(string) (*user.User, error)) (string, error) {
	if name == "" || strings.ContainsAny(name, "\r\n\x00") {
		return "", fmt.Errorf("autologin requires an identified non-root user; run Tailor through sudo or doas")
	}
	account, err := lookup(name)
	if err != nil {
		return "", fmt.Errorf("autologin user %q: %w", name, err)
	}
	if account.Uid == "0" {
		return "", fmt.Errorf("autologin requires a non-root user; run Tailor through sudo or doas")
	}
	return account.Username, nil
}
