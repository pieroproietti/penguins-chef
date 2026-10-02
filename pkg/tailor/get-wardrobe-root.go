package tailor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// getWardrobeRoot returns ~/.wardrobe for the "real" user behind this
// elevated process.
//
// Priority order:
// 1. SUDO_USER  -- set by sudo, identifies the calling user reliably.
// 2. logname    -- reads the kernel audit loginuid; survives any number
//                  of 'su' invocations, unlike environment variables that
//                  each elevation mechanism may or may not preserve.
//                  Needed for distros without sudo (e.g. Quirinux/Devuan)
//                  where 'su' is the normal way to become root and
//                  os.UserHomeDir() returns /root instead of the real home.
// 3. firstHumanUser -- scan /etc/passwd for the first UID 1000-59999 with
//                       a valid login shell, last resort before giving up.
// 4. os.UserHomeDir() -- process HOME, works when not elevated at all.
func getWardrobeRoot() (string, error) {
	var homeDir string

	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			homeDir = u.HomeDir
		}
	}

	if homeDir == "" {
		if out, err := exec.Command("logname").Output(); err == nil {
			login := strings.TrimSpace(string(out))
			if login != "" && login != "root" {
				if u, err := user.Lookup(login); err == nil {
					homeDir = u.HomeDir
				}
			}
		}
	}

	if homeDir == "" {
		if u := firstHumanUser(); u != nil {
			homeDir = u.HomeDir
		}
	}

	if homeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to determine home directory: %v", err)
		}
		homeDir = home
	}

	return filepath.Join(homeDir, ".wardrobe"), nil
}

// getWardrobeV2Dir returns ~/.wardrobe/v2 for the "real" user if present, or ~/.wardrobe,
// falling back to local working directory ./v2 if present during development.
func getWardrobeV2Dir() (string, error) {
	root, err := getWardrobeRoot()
	if err == nil {
		v2 := filepath.Join(root, "v2")
		if _, errStat := os.Stat(v2); errStat == nil {
			return v2, nil
		}
		if _, errStat := os.Stat(root); errStat == nil {
			return root, nil
		}
	}

	// Fallback to local working directory ./v2 if developing in repo
	if stat, errStat := os.Stat("v2"); errStat == nil && stat.IsDir() {
		if abs, errAbs := filepath.Abs("v2"); errAbs == nil {
			return abs, nil
		}
		return "v2", nil
	}

	return root, nil
}

// firstHumanUser scans /etc/passwd for the first real (non-system) user:
// UID between 1000 and 59999 with a valid login shell.
func firstHumanUser() *user.User {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid >= 60000 {
			continue
		}
		shell := fields[6]
		if strings.HasSuffix(shell, "nologin") || strings.HasSuffix(shell, "/false") {
			continue
		}
		if u, err := user.Lookup(fields[0]); err == nil {
			return u
		}
	}
	return nil
}

// GetWardrobeOrigin returns the remote origin URL of ~/.wardrobe/.git if available.
func GetWardrobeOrigin() string {
	root, err := getWardrobeRoot()
	if err == nil {
		if origin := getGitOrigin(root); origin != "" {
			return origin
		}
	}
	// Fallback to local directory if developing inside a wardrobe repo
	return getGitOrigin(".")
}

// GetWardrobeBranch returns the active git branch of ~/.wardrobe/.git if available.
func GetWardrobeBranch() string {
	root, err := getWardrobeRoot()
	if err == nil {
		if branch := getGitBranch(root); branch != "" {
			return branch
		}
	}
	return getGitBranch(".")
}

// getGitBranch extracts current branch name from a directory containing a .git repository.
func getGitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return ""
	}

	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" && branch != "HEAD" {
			return branch
		}
	}

	// Fallback to parsing .git/HEAD
	headFile := filepath.Join(gitDir, "HEAD")
	data, err := os.ReadFile(headFile)
	if err == nil {
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "ref: refs/heads/") {
			return strings.TrimPrefix(content, "ref: refs/heads/")
		}
	}

	return ""
}


// getGitOrigin extracts the remote origin URL from a directory containing a .git repository.
func getGitOrigin(dir string) string {
	if dir == "" {
		return ""
	}
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return ""
	}

	// Try using git command first
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err == nil {
		url := strings.TrimSpace(string(out))
		if url != "" {
			return url
		}
	}

	// Fallback to parsing .git/config directly
	configFile := filepath.Join(gitDir, "config")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return ""
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inOriginSection := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inOriginSection = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}
		if inOriginSection && strings.HasPrefix(line, "url") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}

// normalizeGitURL normalizes a Git URL for comparison.
func normalizeGitURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "#"); idx != -1 {
		raw = raw[:idx]
	}
	raw = strings.TrimSuffix(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")

	for _, scheme := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(strings.ToLower(raw), scheme) {
			raw = raw[len(scheme):]
			break
		}
	}
	if strings.HasPrefix(raw, "git@") {
		raw = raw[4:]
		raw = strings.Replace(raw, ":", "/", 1)
	}
	if idx := strings.Index(raw, "@"); idx != -1 {
		raw = raw[idx+1:]
	}
	raw = strings.TrimSuffix(raw, "/")
	return strings.ToLower(raw)
}
