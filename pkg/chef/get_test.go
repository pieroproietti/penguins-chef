package chef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://github.com/pieroproietti/penguins-chef",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "https://github.com/pieroproietti/penguins-chef.git",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "https://github.com/pieroproietti/penguins-chef/",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "git@github.com:pieroproietti/penguins-chef.git",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "http://github.com/pieroproietti/penguins-chef",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "ssh://git@github.com/pieroproietti/penguins-chef.git",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "https://github.com/charliemartinez/penguins-chef",
			expected: "github.com/charliemartinez/penguins-chef",
		},
		{
			input:    "https://github.com/pieroproietti/penguins-chef#main",
			expected: "github.com/pieroproietti/penguins-chef",
		},
		{
			input:    "https://github.com/pieroproietti/penguins-chef.git#dev",
			expected: "github.com/pieroproietti/penguins-chef",
		},
	}

	for _, tt := range tests {
		got := normalizeGitURL(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeGitURL(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}

	// Verify equality of equivalents
	u1 := "https://github.com/pieroproietti/penguins-chef"
	u2 := "git@github.com:pieroproietti/penguins-chef.git"
	u3 := "https://github.com/charliemartinez/penguins-chef"

	if normalizeGitURL(u1) != normalizeGitURL(u2) {
		t.Errorf("expected %q and %q to normalize equally", u1, u2)
	}

	if normalizeGitURL(u1) == normalizeGitURL(u3) {
		t.Errorf("expected %q and %q to normalize differently", u1, u3)
	}
}

func TestGetGitOrigin(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existent dir or non-git dir
	if origin := getGitOrigin(tempDir); origin != "" {
		t.Errorf("expected empty origin for non-git dir, got %q", origin)
	}

	// 2. Git dir with .git/config
	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
[remote "origin"]
	url = https://github.com/pieroproietti/penguins-chef.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write .git/config: %v", err)
	}

	origin := getGitOrigin(tempDir)
	expected := "https://github.com/pieroproietti/penguins-chef.git"
	if origin != expected {
		t.Errorf("getGitOrigin() = %q, expected %q", origin, expected)
	}
}

func TestGetGitBranch(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existent dir or non-git dir
	if branch := getGitBranch(tempDir); branch != "" {
		t.Errorf("expected empty branch for non-git dir, got %q", branch)
	}

	// 2. Git dir with .git/HEAD pointing to develop
	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/develop\n"), 0644); err != nil {
		t.Fatalf("failed to write .git/HEAD: %v", err)
	}

	branch := getGitBranch(tempDir)
	if branch != "develop" {
		t.Errorf("getGitBranch() = %q, expected 'develop'", branch)
	}
}

func TestGetChefRoot(t *testing.T) {
	root, err := GetChefRoot()
	if err != nil {
		t.Fatalf("GetChefRoot() returned error: %v", err)
	}
	if !strings.HasSuffix(root, ".chef") {
		t.Errorf("GetChefRoot() = %q, expected to end with '.chef'", root)
	}
}
