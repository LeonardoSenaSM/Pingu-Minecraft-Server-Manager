package commands

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type Risk int

const (
	RiskNormal Risk = iota
	RiskSensitive
	RiskDangerous
)

type Command struct {
	Syntax         string
	CategoryKey    string
	DescriptionKey string
	Risk           Risk
}

var catalog = []Command{
	{"help [command]", "cmd_cat_maintenance", "cmd_desc_help", RiskNormal},
	{"say <message>", "cmd_cat_communication", "cmd_desc_say", RiskNormal},
	{"tell <player> <message>", "cmd_cat_communication", "cmd_desc_tell", RiskNormal},
	{"list", "cmd_cat_players", "cmd_desc_list", RiskNormal},
	{"op <player>", "cmd_cat_players", "cmd_desc_op", RiskSensitive},
	{"deop <player>", "cmd_cat_players", "cmd_desc_deop", RiskSensitive},
	{"kick <player> [reason]", "cmd_cat_moderation", "cmd_desc_kick", RiskSensitive},
	{"ban <player> [reason]", "cmd_cat_moderation", "cmd_desc_ban", RiskDangerous},
	{"ban-ip <address> [reason]", "cmd_cat_moderation", "cmd_desc_ban_ip", RiskDangerous},
	{"pardon <player>", "cmd_cat_moderation", "cmd_desc_pardon", RiskSensitive},
	{"pardon-ip <address>", "cmd_cat_moderation", "cmd_desc_pardon_ip", RiskSensitive},
	{"banlist [players|ips]", "cmd_cat_moderation", "cmd_desc_banlist", RiskNormal},
	{"whitelist on", "cmd_cat_access", "cmd_desc_whitelist_on", RiskSensitive},
	{"whitelist off", "cmd_cat_access", "cmd_desc_whitelist_off", RiskSensitive},
	{"whitelist add <player>", "cmd_cat_access", "cmd_desc_whitelist_add", RiskSensitive},
	{"whitelist remove <player>", "cmd_cat_access", "cmd_desc_whitelist_remove", RiskSensitive},
	{"whitelist list", "cmd_cat_access", "cmd_desc_whitelist_list", RiskNormal},
	{"whitelist reload", "cmd_cat_access", "cmd_desc_whitelist_reload", RiskNormal},
	{"gamemode <mode> [player]", "cmd_cat_gameplay", "cmd_desc_gamemode", RiskNormal},
	{"gamemode survival <player>", "cmd_cat_gameplay", "cmd_desc_gamemode", RiskNormal},
	{"gamemode creative <player>", "cmd_cat_gameplay", "cmd_desc_gamemode", RiskNormal},
	{"difficulty peaceful", "cmd_cat_world", "cmd_desc_difficulty", RiskNormal},
	{"difficulty easy", "cmd_cat_world", "cmd_desc_difficulty", RiskNormal},
	{"difficulty normal", "cmd_cat_world", "cmd_desc_difficulty", RiskNormal},
	{"difficulty hard", "cmd_cat_world", "cmd_desc_difficulty", RiskNormal},
	{"difficulty <difficulty>", "cmd_cat_world", "cmd_desc_difficulty", RiskNormal},
	{"weather clear", "cmd_cat_world", "cmd_desc_weather", RiskNormal},
	{"weather rain", "cmd_cat_world", "cmd_desc_weather", RiskNormal},
	{"weather thunder", "cmd_cat_world", "cmd_desc_weather", RiskNormal},
	{"weather <type> [duration]", "cmd_cat_world", "cmd_desc_weather", RiskNormal},
	{"time set day", "cmd_cat_world", "cmd_desc_time", RiskNormal},
	{"time set night", "cmd_cat_world", "cmd_desc_time", RiskNormal},
	{"time set <value>", "cmd_cat_world", "cmd_desc_time", RiskNormal},
	{"save-all", "cmd_cat_maintenance", "cmd_desc_save_all", RiskNormal},
	{"save-off", "cmd_cat_maintenance", "cmd_desc_save_off", RiskSensitive},
	{"save-on", "cmd_cat_maintenance", "cmd_desc_save_on", RiskNormal},
	{"stop", "cmd_cat_maintenance", "cmd_desc_stop", RiskDangerous},
	{"reload confirm", "cmd_cat_maintenance", "cmd_desc_reload", RiskDangerous},
	{"seed", "cmd_cat_world", "cmd_desc_seed", RiskNormal},
	{"tp <source> <destination>", "cmd_cat_gameplay", "cmd_desc_tp", RiskNormal},
	{"give <player> <item> [amount]", "cmd_cat_gameplay", "cmd_desc_give", RiskNormal},
	{"effect give <player> <effect> [duration] [amplifier]", "cmd_cat_gameplay", "cmd_desc_effect", RiskNormal},
	{"gamerule <rule> [value]", "cmd_cat_world", "cmd_desc_gamerule", RiskSensitive},
}

func All() []Command {
	return append([]Command(nil), catalog...)
}

func Filter(query string, translate func(string) string) []Command {
	needle := strings.ToLower(strings.TrimSpace(query))
	if translate == nil {
		translate = func(value string) string { return value }
	}
	result := make([]Command, 0, len(catalog))
	for _, command := range catalog {
		haystack := strings.ToLower(strings.Join([]string{
			command.Syntax, translate(command.CategoryKey), translate(command.DescriptionKey),
		}, " "))
		if needle == "" || strings.Contains(haystack, needle) {
			result = append(result, command)
		}
	}
	return result
}

// SelectionText intentionally returns text for the editable console field. It
// never sends or executes a command.
func SelectionText(command Command) string { return command.Syntax }

func RiskForInput(input string) Risk {
	input = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/")))
	risk := RiskNormal
	for _, command := range catalog {
		literal := command.Syntax
		if index := strings.Index(literal, "<"); index >= 0 {
			literal = literal[:index]
		}
		if index := strings.Index(literal, "["); index >= 0 {
			literal = literal[:index]
		}
		literal = strings.ToLower(strings.TrimSpace(literal))
		if input == literal || strings.HasPrefix(input, literal+" ") {
			if command.Risk > risk {
				risk = command.Risk
			}
		}
	}
	return risk
}

func ValidateInput(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("comando vazio")
	}
	if !utf8.ValidString(command) || strings.ContainsAny(command, "\r\n\x00") {
		return errors.New("comando contém caracteres inválidos")
	}
	if len(command) > 2048 {
		return errors.New("comando excede 2048 bytes")
	}
	return nil
}
