package i18n

import (
	"fmt"
	"sync"
)

const (
	English    = "en"
	Portuguese = "pt-BR"
)

type Localizer struct {
	mu       sync.RWMutex
	language string
}

func New(language string) *Localizer {
	localizer := &Localizer{}
	localizer.SetLanguage(language)
	return localizer
}

func (l *Localizer) SetLanguage(language string) {
	if _, exists := messages[language]; !exists {
		language = Portuguese
	}
	l.mu.Lock()
	l.language = language
	l.mu.Unlock()
}

func (l *Localizer) Language() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.language
}

func (l *Localizer) T(key string, args ...any) string {
	l.mu.RLock()
	language := l.language
	l.mu.RUnlock()
	value := messages[language][key]
	if value == "" {
		value = messages[English][key]
	}
	if value == "" {
		value = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(value, args...)
	}
	return value
}

var messages = map[string]map[string]string{
	English: {
		"home_tab": "Home & Status", "console_tab": "Terminal / Console", "players_tab": "Player Manager", "controls_tab": "Server Controls", "settings_tab": "Settings & Files",
		"stopped": "Stopped", "starting": "Starting", "running": "Running", "standby": "Standby", "listening": "Listening", "inactive": "Inactive",
		"waiting_address": "Click PLAY to detect LAN addresses and reserve free ports.", "logs_placeholder": "Paper, plugins and direct network proxy logs will appear here.", "file_placeholder": "Select a file to preview its contents.",
		"jvm_title": "JVM / Paper", "jvm_subtitle": "Server status", "network_title": "Direct / LAN Network", "network_subtitle": "TCP and UDP listeners", "connection_title": "Connection addresses", "connection_subtitle": "Java + Bedrock",
		"links_title": "Connection Links", "links_subtitle": "Live LAN endpoints. Forward these same ports on your router for Internet access.", "java_edition": "Java Edition", "java_protocol": "TCP direct / on-demand", "bedrock_crossplay": "Bedrock Crossplay", "bedrock_protocol": "UDP via Geyser", "supported_version": "Supported version: %s", "bedrock_supported_range": "v1.20.x–v1.21.x via Geyser", "version_not_installed": "not installed", "link_on": "● ON", "link_off": "○ OFF", "link_waiting": "Waiting for PLAY", "copy_link": "Copy Link", "link_copied": "✓ Copied", "shutdown_links": "Shutdown Links", "shutdown_confirm_title": "Shutdown connection links?", "shutdown_confirm_message": "Active Java and Bedrock connections will be closed immediately and both ports will be released. Paper remains available for a later PLAY.", "shutdown_failed": "Unable to shut down links", "shutdown_complete": "Links shut down", "shutdown_complete_message": "The TCP/UDP proxies were stopped and their ports were released.",
		"play": "PLAY", "stop_server": "Stop Server", "update_dependencies": "Update Dependencies", "quick_guide": "ON-DEMAND QUICK GUIDE\n\n1. Click PLAY and wait for first-time downloads.\n2. Share a displayed LAN address with local players.\n3. For Internet access, forward the displayed Java TCP and Bedrock UDP ports to this computer and allow them through Windows Firewall.\n4. The Paper server starts on the first packet and returns to standby after 15 minutes without players or traffic.",
		"command_placeholder": "Enter a command: say Hello, time set day, save-all...", "send": "Send",
		"online_players": "Online Players", "offline_players": "Offline Players", "banned_players": "Banned Players", "quick_actions": "Direct actions", "known_players": "Known players", "ban_details": "Name/IP, date and expiration", "no_online": "No players are online.", "no_offline": "No offline players are known yet.", "no_banned": "No players or IP addresses are banned.",
		"make_op": "Make OP / Admin", "kick": "Kick", "tempban": "Tempban 3 days", "pardon": "Pardon", "banned_at": "Banned at %s", "expires_at": "expires at %s",
		"minecraft_version": "Minecraft Version", "version_subtitle": "Choose a stable PaperMC version", "installed_version": "Installed version: %s", "unknown": "unknown", "apply_version": "Download / Apply Version", "refresh_list": "Refresh List", "latest_stable": "Latest stable", "version_warning": "Stop the server before changing versions. Back up the world before downgrading.",
		"general": "General", "general_subtitle": "World administration", "save_world": "Save World", "weather": "Weather", "weather_subtitle": "Instant control", "clear": "Clear", "rain": "Rain", "thunder": "Thunderstorm", "time": "Time", "time_subtitle": "Day or night", "day": "Day", "night": "Night", "difficulty": "Difficulty", "difficulty_subtitle": "World level", "peaceful": "Peaceful", "easy": "Easy", "normal": "Normal", "hard": "Hard", "gamemode": "Game Mode", "gamemode_subtitle": "Applied to the player entered below", "player_name": "Exact Minecraft player name", "survival": "Survival", "creative": "Creative",
		"language": "Language / Idioma", "english": "English", "portuguese": "Português (Brasil)", "server_name": "Server Name / Hostname", "server_name_placeholder": "Local instance name", "max_players": "Maximum players (server.properties)", "max_players_placeholder": "Example: 20", "save_settings": "Save Settings", "settings_title": "Application Settings", "settings_subtitle": "Identity, player limit and language",
		"files_title": "Server File Explorer", "files_subtitle": "Contents of ./server/", "open_folder": "Open Folder", "empty_folder": "The server folder is empty.", "folder": "Folder: %s", "preview_error": "Unable to preview the file:\n%s",
		"start_failed": "Unable to start", "stop_failed": "Unable to stop", "update_failed": "Update interrupted", "updated_title": "Dependencies updated", "updated_message": "Paper and plugins were downloaded successfully.", "open_failed": "Unable to open", "command_failed": "Command not sent", "promote_failed": "Unable to grant OP", "kick_failed": "Unable to kick player", "ban_failed": "Unable to ban player", "pardon_failed": "Unable to pardon player", "list_failed": "Unable to list files", "versions_failed": "Unable to query PaperMC versions", "apply_failed": "Unable to apply version", "version_applied": "Version applied", "version_ready": "Paper %s is ready for the next start.",
		"name_required": "Player name required", "name_required_message": "Enter the exact Minecraft player name.", "settings_saved": "Settings saved", "settings_saved_message": "Server name, max players and language were saved.", "settings_failed": "Unable to save settings", "invalid_settings": "Enter a server name and max-players from 1 to 1000.", "loading": "Loading...", "progress_idle": "Ready", "progress_working": "Working: %s", "cancel": "Cancel", "cancelled": "Operation cancelled",
	},
	Portuguese: {
		"home_tab": "Início & Status", "console_tab": "Terminal / Console", "players_tab": "Gerenciador de Jogadores", "controls_tab": "Controles do Servidor", "settings_tab": "Configurações & Arquivos",
		"stopped": "Parado", "starting": "Iniciando", "running": "Em Execução", "standby": "Em Espera", "listening": "Escutando", "inactive": "Inativa",
		"waiting_address": "Clique em PLAY para detectar os endereços LAN e reservar portas livres.", "logs_placeholder": "Os logs do Paper, plugins e proxy de rede direta aparecerão aqui.", "file_placeholder": "Selecione um arquivo para visualizar seu conteúdo.",
		"jvm_title": "JVM / Paper", "jvm_subtitle": "Estado do servidor", "network_title": "Rede Direta / LAN", "network_subtitle": "Listeners TCP e UDP", "connection_title": "Endereços de conexão", "connection_subtitle": "Java + Bedrock",
		"links_title": "Links de Conexão", "links_subtitle": "Endpoints LAN em tempo real. Para acesso pela Internet, redirecione estas mesmas portas no roteador.", "java_edition": "Java Edition", "java_protocol": "TCP direto / sob demanda", "bedrock_crossplay": "Bedrock Crossplay", "bedrock_protocol": "UDP via Geyser", "supported_version": "Versão suportada: %s", "bedrock_supported_range": "v1.20.x–v1.21.x via Geyser", "version_not_installed": "não instalada", "link_on": "● ON", "link_off": "○ OFF", "link_waiting": "Aguardando PLAY", "copy_link": "Copiar Link", "link_copied": "✓ Copiado", "shutdown_links": "Desligar Links", "shutdown_confirm_title": "Desligar os links de conexão?", "shutdown_confirm_message": "As conexões Java e Bedrock ativas serão encerradas imediatamente e as duas portas serão liberadas. O Paper continuará disponível para um próximo PLAY.", "shutdown_failed": "Falha ao desligar links", "shutdown_complete": "Links desligados", "shutdown_complete_message": "Os proxies TCP/UDP foram encerrados e suas portas foram liberadas.",
		"play": "PLAY", "stop_server": "Parar Servidor", "update_dependencies": "Atualizar Dependências", "quick_guide": "GUIA RÁPIDO SOB DEMANDA\n\n1. Clique em PLAY e aguarde os downloads da primeira execução.\n2. Compartilhe um dos endereços LAN exibidos com jogadores da rede local.\n3. Para acesso pela Internet, redirecione no roteador as portas TCP do Java e UDP do Bedrock para este computador e libere-as no Firewall do Windows.\n4. O Paper inicia no primeiro pacote e volta à espera após 15 minutos sem jogadores ou tráfego.",
		"command_placeholder": "Digite um comando: say Olá, time set day, save-all...", "send": "Enviar",
		"online_players": "Jogadores Online", "offline_players": "Jogadores Offline", "banned_players": "Jogadores Banidos", "quick_actions": "Ações diretas", "known_players": "Jogadores conhecidos", "ban_details": "Nome/IP, data e expiração", "no_online": "Nenhum jogador conectado.", "no_offline": "Nenhum jogador offline conhecido ainda.", "no_banned": "Nenhum jogador ou IP banido.",
		"make_op": "Tornar OP / ADM", "kick": "Expulsar", "tempban": "Banir por 3 dias", "pardon": "Desbanir", "banned_at": "Banido em %s", "expires_at": "expira em %s",
		"minecraft_version": "Versão do Minecraft", "version_subtitle": "Escolha uma versão estável do PaperMC", "installed_version": "Versão instalada: %s", "unknown": "não identificada", "apply_version": "Baixar / Aplicar Versão", "refresh_list": "Atualizar Lista", "latest_stable": "Mais recente estável", "version_warning": "Pare o servidor antes de trocar versões. Faça backup do mundo antes de retornar para uma versão anterior.",
		"general": "Geral", "general_subtitle": "Administração do mundo", "save_world": "Salvar Mundo", "weather": "Clima", "weather_subtitle": "Controle instantâneo", "clear": "Ensolarado", "rain": "Chuva", "thunder": "Tempestade", "time": "Horário", "time_subtitle": "Dia ou noite", "day": "Dia", "night": "Noite", "difficulty": "Dificuldade", "difficulty_subtitle": "Nível do mundo", "peaceful": "Pacífico", "easy": "Fácil", "normal": "Normal", "hard": "Difícil", "gamemode": "Modo de Jogo", "gamemode_subtitle": "Aplicado ao jogador informado abaixo", "player_name": "Nome exato do jogador no Minecraft", "survival": "Sobrevivência", "creative": "Criativo",
		"language": "Language / Idioma", "english": "English", "portuguese": "Português (Brasil)", "server_name": "Nome do Servidor / Hostname", "server_name_placeholder": "Nome da instância local", "max_players": "Máximo de jogadores (server.properties)", "max_players_placeholder": "Exemplo: 20", "save_settings": "Salvar Configurações", "settings_title": "Configurações do Aplicativo", "settings_subtitle": "Identidade, limite de jogadores e idioma",
		"files_title": "Explorador de Arquivos do Servidor", "files_subtitle": "Conteúdo de ./server/", "open_folder": "Abrir Pasta", "empty_folder": "A pasta server está vazia.", "folder": "Pasta: %s", "preview_error": "Não foi possível visualizar o arquivo:\n%s",
		"start_failed": "Não foi possível iniciar", "stop_failed": "Falha ao parar", "update_failed": "Atualização interrompida", "updated_title": "Dependências atualizadas", "updated_message": "Paper e plugins foram baixados com sucesso.", "open_failed": "Falha ao abrir", "command_failed": "Comando não enviado", "promote_failed": "Falha ao promover", "kick_failed": "Falha ao expulsar", "ban_failed": "Falha ao banir", "pardon_failed": "Falha ao desbanir", "list_failed": "Falha ao listar arquivos", "versions_failed": "Falha ao consultar versões do PaperMC", "apply_failed": "Não foi possível aplicar a versão", "version_applied": "Versão aplicada", "version_ready": "O Paper %s está pronto para a próxima inicialização.",
		"name_required": "Nome necessário", "name_required_message": "Informe o nome exato do jogador no Minecraft.", "settings_saved": "Configurações salvas", "settings_saved_message": "O nome, max-players e idioma foram salvos.", "settings_failed": "Falha ao salvar configurações", "invalid_settings": "Informe um nome de servidor e max-players entre 1 e 1000.", "loading": "Carregando...", "progress_idle": "Pronto", "progress_working": "Executando: %s", "cancel": "Cancelar", "cancelled": "Operação cancelada",
	},
}
