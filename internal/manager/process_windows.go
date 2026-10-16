//go:build windows

package manager

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureChildProcess impede que o Windows crie uma janela de console
// separada para o java.exe. O pingu.exe é compilado com -H=windowsgui e,
// portanto, não possui console; sem esta flag, o Windows abre um console
// vazio (a saída é redirecionada ao Pingu). Fechar essa janela envia
// CTRL_CLOSE_EVENT à JVM, que termina com STATUS_CONTROL_C_EXIT
// (0xC000013A) sem salvar o mundo pelo fluxo normal de "stop".
func configureChildProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
