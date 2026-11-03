package helper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemPredicatesReleaseResources(t *testing.T) {
	for _, predicate := range []struct {
		name                     string
		call                     func(string) bool
		file, directory, missing bool
	}{
		{"IsExist", IsExist, true, true, false},
		{"IsNotExist", IsNotExist, false, false, true},
		{"IsDir", IsDir, false, true, false},
	} {
		for _, kind := range []string{"file", "directory", "missing"} {
			t.Run(predicate.name+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "entry")
				want := predicate.missing
				switch kind {
				case "file":
					if err := os.WriteFile(path, []byte("resource"), 0600); err != nil {
						t.Fatal(err)
					}
					want = predicate.file
				case "directory":
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					want = predicate.directory
				}
				if got := predicate.call(path); got != want {
					t.Fatalf("predicate=%t want=%t", got, want)
				}
				if kind != "missing" {
					if err := os.Remove(path); err != nil {
						t.Fatalf("predicate retained resource: %v", err)
					}
				}
				if err := os.Remove(root); err != nil {
					t.Fatalf("predicate retained directory: %v", err)
				}
			})
		}
	}
}
