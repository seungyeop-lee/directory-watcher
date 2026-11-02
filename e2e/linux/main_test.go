package main

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotUsesWorkingFilesAndOmitsDeletedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tracked.go"), []byte("dirty tracked source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.go"), []byte("untracked source"), 0644); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := snapshot(&data, root, []string{"tracked.go", "new.go", "deleted.go", ""}); err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(&data)
	got := map[string]string{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		got[header.Name] = string(content)
	}
	if len(got) != 2 || got["src/tracked.go"] != "dirty tracked source" || got["src/new.go"] != "untracked source" {
		t.Fatalf("snapshot content: %v", got)
	}
}
