package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// asyncTasks owns the single operation slot. Cancelled tasks retain their ID
// until a replacement starts, so their completion can still leave busy state.
type asyncTasks struct {
	operation int
	cancel    context.CancelFunc
}

type asyncTask struct {
	id     int
	ctx    context.Context
	cancel context.CancelFunc
}

func (r *asyncTasks) begin(ctx context.Context, timeout time.Duration) asyncTask {
	r.stop()
	r.operation++
	ctx, r.cancel = context.WithTimeout(ctx, timeout)
	return asyncTask{id: r.operation, ctx: ctx, cancel: r.cancel}
}

func (r *asyncTasks) stop() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func (t asyncTask) command(run func(context.Context, int) tea.Msg) tea.Cmd {
	return func() tea.Msg { defer t.cancel(); return run(t.ctx, t.id) }
}

func (m *tuiModel) beginOperation(activity string) asyncTask {
	task := m.asyncTasks.begin(m.ctx, m.timeout)
	m.busy, m.meter, m.activity = true, nil, activity
	m.status, m.statusError = "", false
	return task
}

type confirmation int

const (
	confirmationPending confirmation = iota
	confirmationAccepted
	confirmationCancelled
)

func confirmKey(key string) confirmation {
	switch key {
	case "y", "Y":
		return confirmationAccepted
	case "n", "N", "enter", "esc":
		return confirmationCancelled
	default:
		return confirmationPending
	}
}

func confirmView(text, action string) string {
	return text + "\n\n[y/N] " + action + " • Enter/Esc cancel"
}

type formAction int

const (
	formEditing formAction = iota
	formSubmitted
	formCancelled
)

// inputFields delegates editing to Bubbles; it only coordinates focus and the
// shared submit/cancel keys for one-field credentials and multi-field dialogs.
type inputFields struct {
	fields []textinput.Model
	field  int
}

func newTextInput(prompt string, width, limit int, hidden bool) textinput.Model {
	input := textinput.New()
	input.Prompt, input.CharLimit = prompt, limit
	input.SetWidth(width)
	if hidden {
		input.EchoMode, input.EchoCharacter = textinput.EchoPassword, '•'
	}
	return input
}

func (f *inputFields) focus(index int) tea.Cmd {
	if len(f.fields) == 0 {
		return nil
	}
	for i := range f.fields {
		f.fields[i].Blur()
	}
	f.field = (index%len(f.fields) + len(f.fields)) % len(f.fields)
	return f.fields[f.field].Focus()
}

func (f *inputFields) update(message tea.Msg) (formAction, tea.Cmd) {
	if len(f.fields) == 0 {
		return formEditing, nil
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc", "ctrl+c":
			return formCancelled, nil
		case "enter":
			if f.field == len(f.fields)-1 {
				return formSubmitted, nil
			}
			return formEditing, f.focus(f.field + 1)
		case "tab":
			return formEditing, f.focus(f.field + 1)
		case "shift+tab":
			return formEditing, f.focus(f.field - 1)
		}
	}
	var cmd tea.Cmd
	f.fields[f.field], cmd = f.fields[f.field].Update(message)
	return formEditing, cmd
}

func (f *inputFields) view() string {
	views := make([]string, len(f.fields))
	for i := range f.fields {
		views[i] = f.fields[i].View()
	}
	return strings.Join(views, "\n")
}
