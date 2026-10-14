package gui

import (
	"net/http"
	"runtime"
)

// Flags identifies the host platform and the operations it exposes to its surface.
// IsMobile is explicit; filesystem access is not a platform identifier.
type Flags struct {
	IsMobile         bool `json:"isMobile"`
	CanMount         bool `json:"canMount"`
	CanBrowseLocalFS bool `json:"canBrowseLocalFS"`
}

func hostFlags() Flags {
	mobile := runtime.GOOS == "android" || runtime.GOOS == "ios"
	return Flags{IsMobile: mobile, CanMount: !mobile, CanBrowseLocalFS: !mobile}
}

func (a *Application) flags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, hostFlags())
}
