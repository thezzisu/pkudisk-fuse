package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPasswordPromptMasksInputAndCancels(t *testing.T) {
	prompt := newCredentialPrompt("Password: ", true)
	prompt.fields[0].Focus()
	prompt.Update(tea.KeyPressMsg{Code: 's', Text: "secret"})
	if prompt.fields[0].Value() != "secret" || strings.Contains(prompt.View().Content, "secret") {
		t.Fatal("password displayed in clear text")
	}
	_, cmd := prompt.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !prompt.cancelled || prompt.submitted || cmd == nil {
		t.Fatal("Ctrl-C did not stop the password prompt")
	}
}
