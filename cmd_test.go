package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFlagDefaults(t *testing.T) {
	command := newGenerateCommand()
	tests := map[string]string{
		"input": defaultInput, "output": defaultOutput, "group-by": defaultGroupBy,
	}
	for name, want := range tests {
		if got := command.Flag(name).DefValue; got != want {
			t.Errorf("--%s default = %q, want %q", name, got, want)
		}
	}
}

func TestGenerateRejectsInvalidGroupBy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newRootCommand(&stdout, &stderr)
	command.SetArgs([]string{"generate", "--group-by", "package"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "must be directory or community") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestGenerateWritesBundle(t *testing.T) {
	tempDir := t.TempDir()
	input := filepath.Join(tempDir, "graph.json")
	if err := os.WriteFile(input, []byte(`{"nodes":[{"id":"one"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(tempDir, "bundle")
	command := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"generate", "-i", input, "-o", output})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "_ungrouped", "one.md")); err != nil {
		t.Fatalf("generated concept: %v", err)
	}
	rootIndex, err := os.ReadFile(filepath.Join(output, "index.md"))
	if err != nil || !strings.Contains(string(rootIndex), `okf_version: "0.2"`) {
		t.Fatalf("root index = %q, error = %v", rootIndex, err)
	}
}
