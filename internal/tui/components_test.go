package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestAsyncReplacementCannotBeCancelledByOldCompletion(t *testing.T) {
	var tasks asyncTasks
	old := tasks.begin(context.Background(), time.Minute)
	current := tasks.begin(context.Background(), time.Minute)
	defer tasks.stop()
	if current.id <= old.id || !errors.Is(old.ctx.Err(), context.Canceled) {
		t.Fatal("replacement retained the old task or reused its identity")
	}
	old.command(func(ctx context.Context, id int) tea.Msg {
		if id != old.id || !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("old command received replacement state")
		}
		return nil
	})()
	if current.ctx.Err() != nil {
		t.Fatal("old completion cancelled the active task")
	}
	tasks.stop()
	if !errors.Is(current.ctx.Err(), context.Canceled) {
		t.Fatal("stop did not cancel the active task")
	}
}

func TestInputFieldsUseOneFocusAndRequireLastFieldToSubmit(t *testing.T) {
	form := inputFields{fields: []textinput.Model{newTextInput("Password: ", 20, 50, true), newTextInput("Name: ", 20, 50, false)}}
	form.focus(0)
	form.update(tea.KeyPressMsg{Code: 's', Text: "secret"})
	if strings.Contains(form.view(), "secret") {
		t.Fatal("password was exposed")
	}
	state, _ := form.update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if state != formEditing || form.field != 1 || form.fields[0].Focused() || !form.fields[1].Focused() {
		t.Fatal("Enter submitted before the last field or left multiple focused inputs")
	}
	form.update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if form.field != 0 {
		t.Fatal("Shift-Tab did not return to the previous field")
	}
	form.update(tea.KeyPressMsg{Code: tea.KeyTab})
	state, _ = form.update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if state != formSubmitted {
		t.Fatal("last field did not submit")
	}
	state, _ = form.update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if state != formCancelled {
		t.Fatal("Escape did not cancel")
	}
}
