package gui

import "time"

// Busy reports whether work still owns application resources, including login,
// incoming uploads before they create a task, and outgoing response cleanup.
// This is a lifetime observation, not a shutdown guard: hosts must call
// BeginShutdown to atomically check for work and prevent new operations.
func (a *Application) Busy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activeWrites > 0 || a.busyLocked("")
}

// TaskInfo is an independent, read-only view of a task for application hosts.
// It contains no credentials, mutable result objects, or presentation policy.
type TaskInfo struct {
	ID          string    `json:"id"`
	ClientID    string    `json:"clientId,omitempty"`
	Kind        string    `json:"kind"`
	Account     string    `json:"account"`
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	State       string    `json:"state"`
	Phase       string    `json:"phase"`
	Done        int64     `json:"done"`
	Total       int64     `json:"total"`
	Percent     int       `json:"percent"`
	Created     time.Time `json:"created"`
	Active      bool      `json:"active"`
}

// Tasks returns a newest-first snapshot, using the latest attempt when browser
// retries or an attachment handoff share a client ID. Active remains true until
// every attempt has released its resources, including its HTTP response. Hosts
// can therefore manage their lifetime without knowing the transfer transport.
// The returned values share no mutable data with the application.
func (a *Application) Tasks() []TaskInfo {
	snapshot := a.snapshotJobs()
	rows := make([]TaskInfo, 0, len(snapshot))
	type attemptKey struct{ account, kind, id string }
	seen := make(map[attemptKey]int)
	for _, item := range snapshot {
		info := item.info
		if info.ClientID != "" && info.Kind != "mount" {
			key := attemptKey{info.Account, info.Kind, info.ClientID}
			if index, found := seen[key]; found {
				rows[index].Active = rows[index].Active || item.active
				continue
			}
			seen[key] = len(rows)
		}
		rows = append(rows, TaskInfo{
			ID: info.ID, ClientID: info.ClientID, Kind: info.Kind, Account: info.Account,
			Name: info.Name, Source: info.Source, Destination: info.Destination,
			State: info.State, Phase: info.Phase, Done: info.Done, Total: info.Total,
			Percent: info.Percent, Created: info.Created, Active: item.active,
		})
	}
	return rows
}
