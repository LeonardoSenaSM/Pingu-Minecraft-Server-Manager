package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var javaVersionPattern = regexp.MustCompile(`(?im)(?:openjdk|java) version "(?:1\.)?([0-9]+)`)

const minimumJavaFeature = 25

func (m *Manager) findJavaExecutable() (string, int, error) {
	binary := "java"
	if runtime.GOOS == "windows" {
		binary = "java.exe"
	}
	candidates := make([]string, 0, 3)
	if absoluteBase, err := filepath.Abs(m.baseDir); err == nil {
		candidates = append(candidates, filepath.Join(absoluteBase, "runtime", "bin", binary))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "runtime", "bin", binary))
	}
	if systemJava, err := exec.LookPath("java"); err == nil {
		candidates = append(candidates, systemJava)
	}

	seen := make(map[string]bool)
	problems := make([]string, 0)
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		key := strings.ToLower(clean)
		if seen[key] {
			continue
		}
		seen[key] = true
		if info, err := os.Stat(clean); err != nil || info.IsDir() {
			continue
		}
		feature, err := inspectJavaFeature(clean)
		if err != nil {
			problems = append(problems, filepath.Base(clean)+": "+err.Error())
			continue
		}
		if feature < minimumJavaFeature {
			problems = append(problems, fmt.Sprintf("%s: Java %d é inferior ao mínimo %d", filepath.Base(clean), feature, minimumJavaFeature))
			continue
		}
		return clean, feature, nil
	}
	detail := ""
	if len(problems) > 0 {
		detail = " (" + strings.Join(problems, "; ") + ")"
	}
	return "", 0, fmt.Errorf("Java %d ou superior não encontrado; reinstale o Pingu completo ou instale um Java compatível%s", minimumJavaFeature, detail)
}

func inspectJavaFeature(javaPath string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	versionCmd := exec.CommandContext(ctx, javaPath, "-version")
	configureChildProcess(versionCmd)
	output, err := versionCmd.CombinedOutput()
	if ctx.Err() != nil {
		return 0, errors.New("tempo limite ao verificar a versão")
	}
	if err != nil {
		return 0, fmt.Errorf("não foi possível executar -version: %w", err)
	}
	return parseJavaFeature(string(output))
}

func parseJavaFeature(output string) (int, error) {
	match := javaVersionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, errors.New("saída de versão não reconhecida")
	}
	feature, err := strconv.Atoi(match[1])
	if err != nil || feature < 1 {
		return 0, errors.New("versão Java inválida")
	}
	return feature, nil
}
