package i18n

import "testing"

func TestLanguagesContainTheSameKeys(t *testing.T) {
	for key := range messages[English] {
		if messages[Portuguese][key] == "" {
			t.Errorf("Portuguese translation missing key %q", key)
		}
	}
	for key := range messages[Portuguese] {
		if messages[English][key] == "" {
			t.Errorf("English translation missing key %q", key)
		}
	}
}

func TestLocalizerFallsBackToPortuguese(t *testing.T) {
	localizer := New("unsupported")
	if localizer.Language() != Portuguese {
		t.Fatalf("language = %q, want %q", localizer.Language(), Portuguese)
	}
}
