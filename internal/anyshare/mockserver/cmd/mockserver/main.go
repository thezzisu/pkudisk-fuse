// mockserver is an explicit test tool. It is never linked into a release binary.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thezzisu/pkudisk-fuse/internal/accounts"
	"github.com/thezzisu/pkudisk-fuse/internal/anyshare"
	"github.com/thezzisu/pkudisk-fuse/internal/anyshare/mockserver"
)

func main() {
	profile := flag.String("profile", "desktop", "desktop or android fixture data")
	flag.Parse()
	name := "campus"
	if *profile == "android" {
		name = "android-e2e"
	}
	server := mockserver.New(name)
	defer server.Close()
	if *profile == "android" {
		server.PutObject(name, mockserver.Object{Entry: anyshare.Entry{ID: mockserver.Library, Name: "Android test library", Size: -1, Dir: true, Type: "user_doc_lib"}})
		data := []byte("Android download test\n")
		server.PutObject(name, mockserver.Object{Entry: anyshare.Entry{ID: mockserver.Library + "/report", Name: "android-report.txt", Size: int64(len(data)), Rev: "1"}, Data: data})
		server.PutObject(name, mockserver.Object{Entry: anyshare.Entry{ID: mockserver.Library + "/unicode", Name: "移动端下载.txt", Size: int64(len(data)), Rev: "1"}, Data: data})
		slow := bytes.Repeat([]byte("background-transfer\n"), (2<<20)/len("background-transfer\n")+1)[:2<<20]
		for id, filename := range map[string]string{"/slow": "background-transfer.bin", "/slow-second": "background-second.bin"} {
			server.PutObject(name, mockserver.Object{Entry: anyshare.Entry{ID: mockserver.Library + id, Name: filename, Size: int64(len(slow)), Rev: "1"}, Data: slow, StreamDelay: 750 * time.Millisecond})
		}
	}
	out := json.NewEncoder(os.Stdout)
	if err := out.Encode(map[string]any{"origin": server.URL, "config": accounts.Config{Version: 1, Default: name, Accounts: []accounts.Account{server.Account(name)}}}); err != nil {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	requests := make(chan []byte)
	go func() {
		defer close(requests)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			select {
			case requests <- append([]byte(nil), scanner.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-requests:
			if !ok {
				return
			}
			var request struct{ Action string }
			if err := json.Unmarshal(data, &request); err != nil || request.Action != "snapshot" {
				_ = out.Encode(map[string]string{"error": "expected snapshot request"})
				continue
			}
			entries := []anyshare.Entry{}
			contents := make(map[string][]byte)
			for _, object := range server.Objects(name) {
				entries = append(entries, object.Entry)
				contents[object.Entry.ID] = object.Data
			}
			if err := out.Encode(map[string]any{"calls": server.Calls(), "entries": entries, "data": contents, "failures": server.Failures()}); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
		}
	}
}
