package manager

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseJavaFeature(t *testing.T) {
	tests := []struct {
		output string
		want   int
	}{
		{`openjdk version "25.0.4" 2026-07-21 LTS`, 25},
		{`openjdk version "26.0.2" 2026-07-21`, 26},
		{`java version "1.8.0_472"`, 8},
	}
	for _, test := range tests {
		got, err := parseJavaFeature(test.output)
		if err != nil {
			t.Fatalf("parseJavaFeature(%q): %v", test.output, err)
		}
		if got != test.want {
			t.Fatalf("parseJavaFeature(%q) = %d, esperado %d", test.output, got, test.want)
		}
	}
}

func TestParseJavaFeatureRejectsUnknownOutput(t *testing.T) {
	if _, err := parseJavaFeature("versão indisponível"); err == nil {
		t.Fatal("saída inválida deveria retornar erro")
	}
}

func TestFindJavaExecutableFailureIsActionable(t *testing.T) {
	t.Setenv("PATH", "")
	m := New(filepath.Join(t.TempDir(), "path with spaces"))
	_, _, err := m.findJavaExecutable()
	if err == nil || !strings.Contains(err.Error(), "Java 25") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindBundledJavaInPathWithSpaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows executable-path case is covered by the release workflow")
	}
	base := filepath.Join(t.TempDir(), "Pingu data with spaces")
	javaPath := filepath.Join(base, "runtime", "bin", "java")
	if err := os.MkdirAll(filepath.Dir(javaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'openjdk version \"25.0.4\"' 1>&2\n"
	if err := os.WriteFile(javaPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	m := New(base)
	gotPath, feature, err := m.findJavaExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != javaPath || feature != 25 {
		t.Fatalf("path=%q feature=%d", gotPath, feature)
	}
}
