package commands

import (
	"testing"

	"mineserver/internal/i18n"
)

func TestFilterBySyntaxAndTranslatedDescription(t *testing.T) {
	if got := Filter("whitelist add", nil); len(got) != 1 || got[0].Syntax != "whitelist add <player>" {
		t.Fatalf("unexpected syntax filter: %+v", got)
	}
	translate := func(key string) string {
		if key == "cmd_desc_weather" {
			return "Controla o clima do mundo"
		}
		return key
	}
	if got := Filter("clima", translate); len(got) != 4 {
		t.Fatalf("translated filter returned %d commands, want 4", len(got))
	}
}

func TestSelectionOnlyReturnsEditableText(t *testing.T) {
	command := All()[0]
	if got := SelectionText(command); got != command.Syntax {
		t.Fatalf("SelectionText = %q, want %q", got, command.Syntax)
	}
}

func TestFilterUsesApplicationTranslations(t *testing.T) {
	portuguese := i18n.New(i18n.Portuguese)
	if got := Filter("clima", func(key string) string { return portuguese.T(key) }); len(got) < 3 {
		t.Fatalf("Portuguese search returned %d weather commands", len(got))
	}
	english := i18n.New(i18n.English)
	if got := Filter("private message", func(key string) string { return english.T(key) }); len(got) != 1 || got[0].Syntax != "tell <player> <message>" {
		t.Fatalf("unexpected English translation search: %+v", got)
	}
}

func TestValidateInput(t *testing.T) {
	for _, invalid := range []string{"", "say hi\nstop", "say \x00"} {
		if err := ValidateInput(invalid); err == nil {
			t.Fatalf("expected %q to be rejected", invalid)
		}
	}
	if err := ValidateInput("/say hello"); err != nil {
		t.Fatal(err)
	}
}

func TestRiskForInput(t *testing.T) {
	if RiskForInput("/ban Alex griefing") != RiskDangerous {
		t.Fatal("ban should be dangerous")
	}
	if RiskForInput("banlist") != RiskNormal {
		t.Fatal("banlist should not be classified as ban")
	}
}
