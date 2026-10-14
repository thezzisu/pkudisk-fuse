package tui

import (
	"context"
	"errors"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type credentialPrompt struct {
	inputFields
	submitted, cancelled bool
}

func newCredentialPrompt(label string, hidden bool) *credentialPrompt {
	return &credentialPrompt{inputFields: inputFields{fields: []textinput.Model{newTextInput(label, 60, 8192, hidden)}}}
}
func (p *credentialPrompt) Init() tea.Cmd { return p.focus(0) }
func (p *credentialPrompt) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := message.(tea.WindowSizeMsg); ok {
		p.fields[0].SetWidth(max(msg.Width-len(p.fields[0].Prompt)-4, 8))
	}
	action, cmd := p.inputFields.update(message)
	switch action {
	case formCancelled:
		p.cancelled = true
		return p, tea.Quit
	case formSubmitted:
		p.submitted = true
		return p, tea.Quit
	}
	return p, cmd
}
func (p *credentialPrompt) View() tea.View { return tea.NewView(p.inputFields.view()) }
func Input(ctx context.Context, label string, hidden bool) ([]byte, error) {
	input, output, err := openTerminal()
	if err != nil {
		return nil, errors.New("interactive terminal unavailable; supply the required command arguments or credential flags")
	}
	defer closeTerminal(input, output)
	prompt := newCredentialPrompt(label, hidden)
	_, err = tea.NewProgram(prompt, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output)).Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if prompt.cancelled || !prompt.submitted {
		return nil, context.Canceled
	}
	value := prompt.fields[0].Value()
	if !hidden {
		value = strings.TrimSpace(value)
	}
	result := []byte(value)
	prompt.fields[0].Reset()
	return result, nil
}

func closeTerminal(input, output *os.File) {
	if input != os.Stdin {
		input.Close()
	}
	if output != input && output != os.Stdout && output != os.Stderr {
		output.Close()
	}
}
