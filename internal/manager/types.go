package manager

import "time"

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
}

type PaperInstallation struct {
	Version      string    `json:"version"`
	DownloadedAt time.Time `json:"downloaded_at"`
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
	Progress func(Progress)
}
