package manager

import (
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

var (
	errProcessExited = errors.New("o servidor não está em execução")
	errStdinBlocked  = errors.New("o servidor não está aceitando comandos (entrada travada)")
)

var stdinWriteTimeout = 5 * time.Second

// javaProcess encapsula uma JVM em execução. A escrita no stdin tem trava
// própria e prazo máximo, para que uma JVM travada nunca bloqueie o lock
// global do Manager (e, com ele, o log e a interface).
type javaProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	started time.Time

	stdinMu sync.Mutex
	stdin   io.WriteCloser
	wedged  bool
}

func newJavaProcess(cmd *exec.Cmd, stdin io.WriteCloser) *javaProcess {
	return &javaProcess{cmd: cmd, stdin: stdin, done: make(chan struct{})}
}

// Done fecha quando o processo termina.
func (p *javaProcess) Done() <-chan struct{} { return p.done }

func (p *javaProcess) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Write envia texto ao console do Paper. Se o pipe não aceitar a escrita no
// prazo, o processo é marcado como travado e as escritas seguintes falham na
// hora, em vez de se acumularem.
func (p *javaProcess) Write(text string) error {
	p.stdinMu.Lock()
	defer p.stdinMu.Unlock()
	if p.exited() {
		return errProcessExited
	}
	if p.wedged {
		return errStdinBlocked
	}
	result := make(chan error, 1)
	go func() {
		_, err := io.WriteString(p.stdin, text)
		result <- err
	}()
	timer := time.NewTimer(stdinWriteTimeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-p.done:
		return errProcessExited
	case <-timer.C:
		p.wedged = true
		return errStdinBlocked
	}
}

// Kill encerra a JVM à força. Usado apenas quando a parada graciosa expira.
func (p *javaProcess) Kill() error {
	if p.cmd == nil || p.cmd.Process == nil || p.exited() {
		return nil
	}
	return p.cmd.Process.Kill()
}

// markExited deve ser chamado uma única vez, depois de cmd.Wait().
func (p *javaProcess) markExited() {
	close(p.done)
	_ = p.stdin.Close()
}
