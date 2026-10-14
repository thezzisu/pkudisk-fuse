package gui

import (
	"context"
	"testing"
)

func TestCoalescedAttemptCancellationAndRemoval(t *testing.T) {
	_, app, s, token := applicationServer(t)
	first, err := app.newJob(jobInfo{Kind: "download", Account: "campus", ClientID: "webview-attempt"}, 0,
		func(ctx context.Context, _ *job) (any, error) { <-ctx.Done(); return nil, ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	newest, err := app.newJob(jobInfo{Kind: "download", Account: "campus", ClientID: first.ClientID}, 0,
		func(context.Context, *job) (any, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	other, err := app.newJob(jobInfo{Kind: "download", Account: "work", ClientID: first.ClientID}, 0,
		func(context.Context, *job) (any, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, app, newest.ID)
	waitJob(t, app, other.ID)
	if w := appRequest(t, s, token, "DELETE", "/api/tasks/"+newest.ID, nil); w.Code != 409 {
		t.Fatalf("removed group with an active older request: %d", w.Code)
	}
	if w := appRequest(t, s, token, "POST", "/api/tasks/"+newest.ID+"/cancel", nil); w.Code != 204 {
		t.Fatalf("cancel attempt: %d %s", w.Code, w.Body.String())
	}
	if finished := waitJob(t, app, first.ID); finished.State != "cancelled" {
		t.Fatalf("older request still alive: %+v", finished)
	}
	app.mu.Lock()
	app.jobs[first.ID].responsePending = true
	app.mu.Unlock()
	if w := appRequest(t, s, token, "DELETE", "/api/tasks/"+newest.ID, nil); w.Code != 409 {
		t.Fatalf("removed group while response owns its resources: %d", w.Code)
	}
	app.mu.Lock()
	app.jobs[first.ID].responsePending = false
	app.mu.Unlock()
	if w := appRequest(t, s, token, "DELETE", "/api/tasks/"+newest.ID, nil); w.Code != 204 {
		t.Fatalf("remove attempt: %d %s", w.Code, w.Body.String())
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if len(app.jobs) != 1 || app.jobs[other.ID] == nil {
		t.Fatal("older rows remain or another account's attempt was removed")
	}
}
