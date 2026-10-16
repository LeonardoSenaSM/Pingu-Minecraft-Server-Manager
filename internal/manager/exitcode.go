package manager

import (
	"errors"
	"fmt"
	"os/exec"
)

// Códigos NTSTATUS relevantes para o encerramento da JVM no Windows.
const (
	statusControlCExit    uint32 = 0xC000013A
	statusAccessViolation uint32 = 0xC0000005
	statusStackOverflow   uint32 = 0xC00000FD
)

// describeProcessExit traduz o erro de cmd.Wait() em uma mensagem que um
// usuário leigo consiga entender, preservando o código original.
func describeProcessExit(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return "processo encerrado com erro"
	}
	code := uint32(exitErr.ExitCode())
	switch code {
	case statusControlCExit:
		return fmt.Sprintf("o Java recebeu um sinal de interrupção (0x%08X): janela de console fechada, Ctrl+C, logoff ou desligamento do Windows; o mundo pode não ter sido salvo", code)
	case statusAccessViolation:
		return fmt.Sprintf("a JVM falhou com violação de acesso (0x%08X); verifique hs_err_pid*.log na pasta do servidor", code)
	case statusStackOverflow:
		return fmt.Sprintf("a JVM falhou por estouro de pilha (0x%08X)", code)
	case 1:
		return "processo encerrado com código 1 (erro do servidor ou finalização forçada); consulte logs/latest.log"
	}
	if code > 0xFFFF {
		return fmt.Sprintf("processo encerrado com código 0x%08X", code)
	}
	return fmt.Sprintf("processo encerrado com código %d", code)
}
