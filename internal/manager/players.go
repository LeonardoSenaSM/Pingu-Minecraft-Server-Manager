package manager

import (
	"bufio"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	joinPattern  = regexp.MustCompile(`:\s+([.A-Za-z0-9_]{1,32}) joined the game`)
	leavePattern = regexp.MustCompile(`:\s+([.A-Za-z0-9_]{1,32}) left the game`)
)

func (m *Manager) consumeProcessOutput(source string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		m.Log("info", source, line, nil)
		m.parseServerLine(line)
	}
	if err := scanner.Err(); err != nil {
		m.Log("error", source, "falha ao ler saída do processo", err)
	}
}

func (m *Manager) parseServerLine(line string) {
	if strings.Contains(line, "Done (") && strings.Contains(line, "For help, type") {
		m.setServerState(StateRunning)
	}
	if match := joinPattern.FindStringSubmatch(line); len(match) == 2 {
		m.mu.Lock()
		m.players[match[1]] = time.Now()
		m.knownPlayers[match[1]] = time.Now()
		m.mu.Unlock()
		m.notifyPlayers()
	}
	if match := leavePattern.FindStringSubmatch(line); len(match) == 2 {
		m.mu.Lock()
		delete(m.players, match[1])
		m.mu.Unlock()
		m.notifyPlayers()
	}
}

func (m *Manager) TempBan(name string, duration time.Duration) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("nome de jogador vazio")
	}
	if err := m.SendCommand("ban " + name + " Banimento temporário de 3 dias"); err != nil {
		return err
	}
	entry := BanEntry{Name: name, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(duration), Reason: "Banimento temporário de 3 dias"}
	m.mu.Lock()
	filtered := m.managedBans[:0]
	for _, previous := range m.managedBans {
		if !strings.EqualFold(previous.Name, name) {
			filtered = append(filtered, previous)
		}
	}
	m.managedBans = append(filtered, entry)
	m.mu.Unlock()
	if err := m.saveManagedBans(); err != nil {
		m.Log("warning", "storage", "não foi possível persistir o banimento", err)
	}
	m.notifyPlayers()
	return nil
}

func (m *Manager) Unban(entry BanEntry) error {
	command := ""
	if entry.Name != "" {
		command = "pardon " + entry.Name
	} else if entry.IP != "" {
		command = "pardon-ip " + entry.IP
	} else {
		return errors.New("registro de banimento inválido")
	}
	if err := m.SendCommand(command); err != nil {
		return err
	}
	m.mu.Lock()
	filtered := m.managedBans[:0]
	for _, previous := range m.managedBans {
		if !(strings.EqualFold(previous.Name, entry.Name) && previous.IP == entry.IP) {
			filtered = append(filtered, previous)
		}
	}
	m.managedBans = filtered
	m.mu.Unlock()
	if err := m.saveManagedBans(); err != nil {
		m.Log("warning", "storage", "não foi possível persistir o desbanimento", err)
	}
	m.notifyPlayers()
	return nil
}

func (m *Manager) banExpiryLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.mu.RLock()
			bans := append([]BanEntry(nil), m.managedBans...)
			running := m.proc != nil
			m.mu.RUnlock()
			if !running {
				continue
			}
			for _, entry := range bans {
				if !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt) {
					if err := m.Unban(entry); err != nil {
						m.Log("error", "players", "falha ao encerrar banimento de "+entry.Name, err)
					}
				}
			}
		}
	}
}
