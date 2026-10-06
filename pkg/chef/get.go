package chef

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pieroproietti/penguins-chef/pkg/utils"
)

const defaultRepoURL = "https://github.com/pieroproietti/penguins-chef"

func Get(repoURL string, branch string) error {
	root, err := GetChefRoot()
	if err != nil {
		return err
	}

	targetURL := repoURL
	if targetURL == "" {
		currentOrigin := getGitOrigin(root)
		if currentOrigin != "" {
			targetURL = currentOrigin
		} else {
			targetURL = defaultRepoURL
		}
	}

	if idx := strings.Index(targetURL, "#"); idx != -1 {
		if branch == "" {
			branch = targetURL[idx+1:]
		}
		targetURL = targetURL[:idx]
	}

	if _, err := os.Stat(root); os.IsNotExist(err) {
		if branch != "" {
			utils.LogNormal("Downloading chef repository from %s (branch: %s)...", targetURL, branch)
			cmd := fmt.Sprintf("git clone -b %s %s %s", branch, targetURL, root)
			if err := utils.Exec(cmd); err != nil {
				return err
			}
		} else {
			utils.LogNormal("Downloading chef repository from %s...", targetURL)
			cmd := fmt.Sprintf("git clone %s %s", targetURL, root)
			if err := utils.Exec(cmd); err != nil {
				return err
			}
		}
		fixChefOwnership(root)
		return nil
	}

	currentOrigin := getGitOrigin(root)
	if currentOrigin != "" && normalizeGitURL(currentOrigin) == normalizeGitURL(targetURL) {
		if branch != "" {
			utils.LogNormal("Chef repository already present in %s (origin: %s). Updating branch %s...", root, currentOrigin, branch)
			cmd := fmt.Sprintf("git -C %s fetch && git -C %s checkout %s && git -C %s pull", root, root, branch, root)
			if err := utils.Exec(cmd); err != nil {
				return err
			}
		} else {
			utils.LogNormal("Chef repository already present in %s (origin: %s). Updating...", root, currentOrigin)
			cmd := fmt.Sprintf("git -C %s pull", root)
			if err := utils.Exec(cmd); err != nil {
				return err
			}
		}
		fixChefOwnership(root)
		return nil
	}

	if currentOrigin != "" {
		utils.LogNormal("Existing chef repository in %s is from %s, switching to %s...", root, currentOrigin, targetURL)
	} else {
		utils.LogNormal("Existing directory in %s is not a valid chef repository, replacing with %s...", root, targetURL)
	}

	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("failed to remove existing chef directory %s: %w", root, err)
	}

	if branch != "" {
		utils.LogNormal("Downloading chef repository from %s (branch: %s)...", targetURL, branch)
		cmd := fmt.Sprintf("git clone -b %s %s %s", branch, targetURL, root)
		if err := utils.Exec(cmd); err != nil {
			return err
		}
	} else {
		utils.LogNormal("Downloading chef repository from %s...", targetURL)
		cmd := fmt.Sprintf("git clone %s %s", targetURL, root)
		if err := utils.Exec(cmd); err != nil {
			return err
		}
	}
	fixChefOwnership(root)
	return nil
}

func fixChefOwnership(root string) {
	if os.Geteuid() != 0 {
		return
	}
	username := os.Getenv("SUDO_USER")
	if username == "" {
		if out, err := exec.Command("logname").Output(); err == nil {
			login := strings.TrimSpace(string(out))
			if login != "" && login != "root" {
				username = login
			}
		}
	}
	if username == "" {
		if u := firstHumanUser(); u != nil {
			username = u.Username
		}
	}
	if username != "" {
		_ = utils.Exec(fmt.Sprintf("chown -R %s:%s %s", username, username, root))
	}
}
