package cmd

import (
	"testing"
)

func TestGetCmdFlags(t *testing.T) {
	cmd := getCmd()

	urlFlag := cmd.Flags().Lookup("url")
	if urlFlag == nil {
		t.Fatal("expected --url flag to exist")
	}
	if urlFlag.Shorthand != "u" {
		t.Errorf("expected shorthand 'u' for --url, got %q", urlFlag.Shorthand)
	}

	branchFlag := cmd.Flags().Lookup("branch")
	if branchFlag == nil {
		t.Fatal("expected --branch flag to exist")
	}
	if branchFlag.Shorthand != "b" {
		t.Errorf("expected shorthand 'b' for --branch, got %q", branchFlag.Shorthand)
	}
}
