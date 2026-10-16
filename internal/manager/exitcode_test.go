package manager

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestDescribeProcessExitNonExitError(t *testing.T) {
	if got := describeProcessExit(errors.New("x")); got != "processo encerrado com erro" {
		t.Fatalf("mensagem inesperada: %q", got)
	}
}

func TestDescribeProcessExitCodeOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("usa sh")
	}
	err := exec.Command("sh", "-c", "exit 1").Run()
	if got := describeProcessExit(err); !strings.Contains(got, "código 1") {
		t.Fatalf("mensagem inesperada: %q", got)
	}
}

func TestDescribeProcessExitControlC(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("STATUS_CONTROL_C_EXIT só pode ser reproduzido no Windows")
	}
	err := exec.Command("cmd", "/c", "exit -1073741510").Run()
	if got := describeProcessExit(err); !strings.Contains(got, "0xC000013A") {
		t.Fatalf("mensagem inesperada: %q", got)
	}
}
