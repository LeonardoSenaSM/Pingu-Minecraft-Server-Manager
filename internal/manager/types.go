package manager

import (
	"fmt"
	"strings"
	"time"
)

const (
	StateStopped  = "stopped"
	StateStarting = "starting"
	StateRunning  = "running"
	StateStandby  = "standby"
)

type Settings struct {
	SelectedMinecraftVersion string `json:"selected_minecraft_version"`
	Language                 string `json:"language"`
	ServerName               string `json:"server_name"`
	MaxPlayers               int    `json:"max_players"`
	MemoryMode               string `json:"memory_mode"`
	MemoryLimitGB            int    `json:"memory_limit_gb,omitempty"`
	// AllowExperimentalBuilds permite usar o build BETA do PaperMC quando a
	// versão escolhida ainda não tem build STABLE. Só é ligado após
	// confirmação explícita do usuário.
	AllowExperimentalBuilds bool `json:"allow_experimental_builds,omitempty"`
}

type PaperInstallation struct {
	Version      string    `json:"version"`
	Channel      string    `json:"channel,omitempty"`
	Build        int       `json:"build,omitempty"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

// Label descreve a instalação para a interface, ex.: "26.3 (beta, build 190)".
func (p PaperInstallation) Label() string {
	if p.Version == "" {
		return ""
	}
	if p.Channel == "" || strings.EqualFold(p.Channel, "STABLE") {
		if p.Build > 0 {
			return fmt.Sprintf("%s (build %d)", p.Version, p.Build)
		}
		return p.Version
	}
	return fmt.Sprintf("%s (%s, build %d)", p.Version, strings.ToLower(p.Channel), p.Build)
}

type BanEntry struct {
	Name      string    `json:"name"`
	IP        string    `json:"ip,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

type FileEntry struct {
	Relative string
	Display  string
	IsDir    bool
}

type Status struct {
	ServerState      string
	NetworkActive    bool
	JavaListening    bool
	BedrockListening bool
	JavaPort         int
	BedrockPort      int
	LocalIPs         []string
	ActiveSessions   int
	ServerName       string
	MinecraftVersion string
	PlayersOnline    int
	MemoryMode       string
	TotalMemoryGB    int
	MemoryMaximumGB  int
	Uptime           time.Duration
}

type LogEntry struct {
	Time       time.Time
	Level      string
	Component  string
	ServerName string
	Message    string
	Err        string
}

func (e LogEntry) String() string {
	line := "[" + e.Time.Format("15:04:05") + "] [" + e.ServerName + "] [" + e.Level + "] [" + e.Component + "] " + e.Message
	if e.Err != "" {
		line += ": " + e.Err
	}
	return line
}

type Progress struct {
	Task          string
	Downloaded    int64
	Total         int64
	Completed     int
	TotalTasks    int
	Indeterminate bool
	Done          bool
	Err           string
}

type Events struct {
	Log      func(LogEntry)
	Status   func(Status)
	Players  func()
	Files    func()
	Language func(string)
	Settings func(Settings)
	Progress func(Progress)
}
