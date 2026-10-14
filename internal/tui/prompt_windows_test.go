package tui

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Run in a private native console, send one Enter per field, and require the
// next prompt to render. A second Enter used to be consumed by the uncancellable
// reader left over from the previous fresh CONIN$ handle.
func TestWindowsCredentialPromptSingleEnter(t *testing.T) {
	if os.Getenv("PKUDISK_TEST_CONSOLE") == "1" {
		runConsolePromptChild(t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestWindowsCredentialPromptSingleEnter$", "-test.v")
	cmd.Env = append(os.Environ(), "PKUDISK_TEST_CONSOLE=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS; child allocates its own console.
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native console prompt failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "single Enter passed") {
		t.Fatal("native console prompt did not finish", string(output))
	}
}

func runConsolePromptChild(t *testing.T) {
	report := os.Stderr
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	result, _, err := kernel.NewProc("AllocConsole").Call()
	if result == 0 {
		fmt.Fprintln(report, "AllocConsole failed:", err)
		t.FailNow()
	}
	defer kernel.NewProc("FreeConsole").Call()
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(report, err)
		t.FailNow()
	}
	defer input.Close()
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(report, err)
		t.FailNow()
	}
	defer output.Close()
	os.Stdin, os.Stdout, os.Stderr = input, output, output
	if err := windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(input.Fd())); err != nil {
		fmt.Fprintln(report, err)
		t.FailNow()
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(output.Fd())); err != nil {
		fmt.Fprintln(report, err)
		t.FailNow()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type outcome struct {
		data []byte
		err  error
	}
	for _, field := range []struct {
		label, value string
		hidden       bool
	}{{"Username: ", "test-user", false}, {"Password: ", "test-secret", true}, {"Verification: ", "123456", true}} {
		done := make(chan outcome, 1)
		go func() { data, err := Input(ctx, field.label, field.hidden); done <- outcome{data, err} }()
		if err := waitConsoleText(ctx, kernel, windows.Handle(output.Fd()), field.label); err != nil {
			fmt.Fprintln(report, err)
			t.FailNow()
		}
		if err := writeConsoleKeys(kernel, windows.Handle(input.Fd()), field.value+"\r"); err != nil {
			fmt.Fprintln(report, err)
			t.FailNow()
		}
		select {
		case got := <-done:
			if got.err != nil || string(got.data) != field.value {
				fmt.Fprintf(report, "prompt %s returned wrong value/error: %v\n", field.label, got.err)
				t.FailNow()
			}
		case <-ctx.Done():
			fmt.Fprintln(report, "prompt needed more than one Enter")
			t.FailNow()
		}
	}
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(input.Fd()), &mode); err != nil {
		fmt.Fprintln(report, "stdin closed after prompt:", err)
		t.FailNow()
	}
	fmt.Fprintln(report, "single Enter passed for username, password and verification")
}

func waitConsoleText(ctx context.Context, kernel *windows.LazyDLL, handle windows.Handle, text string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var buffer [4096]uint16
		var count uint32
		result, _, err := kernel.NewProc("ReadConsoleOutputCharacterW").Call(uintptr(handle), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, uintptr(unsafe.Pointer(&count)))
		if result == 0 {
			return fmt.Errorf("read native prompt: %w", err)
		}
		if strings.Contains(string(utf16.Decode(buffer[:count])), strings.TrimSpace(text)) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func writeConsoleKeys(kernel *windows.LazyDLL, handle windows.Handle, value string) error {
	// INPUT_RECORD is 20 bytes: event type (WORD), padding, KEY_EVENT_RECORD.
	type record struct {
		kind    uint16
		padding uint16
		event   [16]byte
	}
	var records []record
	for _, ch := range utf16.Encode([]rune(value)) {
		for _, down := range []bool{true, false} {
			item := record{kind: 1}
			if down {
				binary.LittleEndian.PutUint32(item.event[0:4], 1)
			}
			binary.LittleEndian.PutUint16(item.event[4:6], 1)
			if ch == '\r' {
				binary.LittleEndian.PutUint16(item.event[6:8], 13)
			}
			binary.LittleEndian.PutUint16(item.event[10:12], ch)
			records = append(records, item)
		}
	}
	var written uint32
	result, _, err := kernel.NewProc("WriteConsoleInputW").Call(uintptr(handle), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)), uintptr(unsafe.Pointer(&written)))
	if result == 0 {
		return err
	}
	if written != uint32(len(records)) {
		return fmt.Errorf("short console input write")
	}
	return nil
}
