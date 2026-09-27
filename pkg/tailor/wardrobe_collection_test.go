package tailor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWardrobeCollection(t *testing.T) {
	for _, tt := range []struct {
		name  string
		dirs  []string
		files []string
		want  string
	}{
		{name: "prefer v3", dirs: []string{"installed/v3", "installed/v2"}, want: "installed/v3"},
		{name: "legacy v2", dirs: []string{"installed/v2"}, want: "installed/v2"},
		{name: "unversioned", dirs: []string{"installed"}, want: "installed"},
		{name: "ignore v3 file", dirs: []string{"installed/v2"}, files: []string{"installed/v3"}, want: "installed/v2"},
		{name: "local v3", dirs: []string{"local/v3", "local/v2"}, want: "local/v3"},
		{name: "local v2", dirs: []string{"local/v2"}, want: "local/v2"},
		{name: "installed takes precedence", dirs: []string{"installed/v2", "local/v3"}, want: "installed/v2"},
		{name: "absent", want: "installed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(base, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, file := range tt.files {
				if err := os.WriteFile(filepath.Join(base, file), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			got := resolveWardrobeCollection(filepath.Join(base, "installed"), filepath.Join(base, "local"))
			if want := filepath.Join(base, tt.want); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
