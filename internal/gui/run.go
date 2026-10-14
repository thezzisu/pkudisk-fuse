package gui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Tray must be constructed and run on the process's main OS thread. Close
// must be safe from another goroutine and cause Run to return.
type Tray interface {
	Run() error
	Close()
}

type Options struct {
	// Ready reports a usable instance after browser activation. The argument is
	// true only when this process owns the service, false for repeat activation.
	Ready func(bool)
	// OnQuit reports an accepted explicit quit request, before shutdown begins.
	// Hosts can distinguish user intent from cancellation or service failure.
	OnQuit         func()
	NewApplication func(context.Context) (*Application, error)
	Version        string
	Directory      string
	Output         io.Writer
	NewTray        func(open, quit func()) (Tray, error)
	OpenBrowser    func(string) error
}

// Run serves a GUI until cancelled or explicitly quit. A second invocation
// opens a fresh login link in the existing instance and returns immediately.
func Run(ctx context.Context, opts Options) error {
	if opts.OpenBrowser == nil || opts.NewTray == nil || opts.Directory == "" {
		return errors.New("missing GUI desktop configuration")
	}
	if opts.Output == nil {
		opts.Output = io.Discard
	}
	devProxy, err := developmentProxy(os.Getenv("PKUDISK_GUI_DEV_PROXY"))
	if err != nil {
		return err
	}
	owner, err := acquire(ctx, opts.Directory)
	if err != nil {
		return err
	}
	if owner == nil {
		fmt.Fprintln(opts.Output, "Opened the interface in the existing GUI instance.")
		if opts.Ready != nil {
			opts.Ready(false)
		}
		return nil
	}
	defer owner.close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	control, err := randomToken()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var app *Application
	if opts.NewApplication != nil {
		app, err = opts.NewApplication(ctx)
		if err != nil {
			return err
		}
		defer app.Close()
	}
	stop := func() {
		if opts.OnQuit != nil {
			opts.OnQuit()
		}
		cancel()
	}
	s := &server{host: listener.Addr().String(), version: opts.Version, control: control, devProxy: devProxy,
		tokens: newTokens(), stop: stop, application: app}
	s.origin = "http://" + s.host
	defer s.tokens.clear()
	// Serialize browser launch requests. The URL contains a credential and is
	// passed only to the browser opener, never to logs or terminal output.
	var opening sync.Mutex
	s.activate = func() error {
		opening.Lock()
		defer opening.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		ticket, err := s.tokens.ticket()
		if err != nil {
			return err
		}
		if err := opts.OpenBrowser(s.origin + "/#ticket=" + ticket); err != nil {
			return errors.New("could not open the default browser; check the desktop browser settings and try again")
		}
		return nil
	}
	var outputMu sync.Mutex
	report := func(err error) {
		outputMu.Lock()
		defer outputMu.Unlock()
		fmt.Fprintln(opts.Output, "GUI:", err)
	}
	var activations sync.WaitGroup
	var activationMu sync.Mutex
	accepting := true
	// Native callbacks must return promptly so the event loop stays responsive.
	// Close/Run join the native callbacks before the final Wait below.
	open := func() {
		activationMu.Lock()
		defer activationMu.Unlock()
		if !accepting {
			return
		}
		activations.Go(func() {
			if err := s.activate(); err != nil && ctx.Err() == nil {
				report(err)
			}
		})
	}
	quit := func() {
		if app != nil {
			if err := app.BeginShutdown(); err != nil {
				report(err)
				open()
				return
			}
		}
		stop()
	}
	tray, trayErr := opts.NewTray(open, quit)
	s.tray = tray != nil
	if trayErr != nil {
		report(trayErr)
	}
	if tray != nil {
		defer tray.Close()
	}
	// Per-request deadlines are applied by Application.ServeHTTP. Keep the
	// server-level body/read/write timeouts open for browser multipart uploads
	// and streamed downloads; their bounded operation contexts still expire.
	httpServer := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 0, WriteTimeout: 0, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 16 << 10}
	served := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		served <- err
		cancel()
	}()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		// First drain handlers (including the quit response); then stop the tray.
		shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if httpServer.Shutdown(shutdown) != nil {
			_ = httpServer.Close()
		}
		if tray != nil {
			tray.Close()
		}
	}()
	defer func() {
		cancel()
		<-stopped
		activationMu.Lock()
		accepting = false
		activationMu.Unlock()
		activations.Wait()
	}()
	if err := owner.publish(instanceInfo{Version: 1, Origin: s.origin, Control: control}); err != nil {
		return err
	}
	// Begin after publishing discovery so another invocation can activate the
	// already-listening instance even while the default browser starts.
	if err := s.activate(); err != nil {
		if tray == nil {
			return errors.New("GUI unavailable: neither a system tray nor a default browser could be opened")
		}
		report(err)
	}
	outputMu.Lock()
	fmt.Fprintln(opts.Output, "GUI running. Close the browser to keep it running; use Exit or Ctrl-C to stop.")
	outputMu.Unlock()
	if opts.Ready != nil {
		opts.Ready(true)
	}
	if tray != nil {
		err = tray.Run()
		cancel()
	} else {
		<-ctx.Done()
	}
	<-stopped
	serveErr := <-served
	if err != nil {
		return err
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}
