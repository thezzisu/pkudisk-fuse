package display

import (
	"bytes"
	"errors"
	"testing"
)

func TestRenderListJSONHasStableEmptyArrays(t *testing.T) {
	var out bytes.Buffer
	if err := RenderList(&out, []string(nil), ListOptions[string]{JSON: true}); err != nil || out.String() != "[]\n" {
		t.Fatal("empty list contract changed", out.String(), err)
	}
	out.Reset()
	err := RenderList(&out, []string(nil), ListOptions[string]{
		JSON: true,
		JSONValue: func(items []string) any {
			return struct {
				Hits  int      `json:"hits"`
				Files []string `json:"files"`
			}{0, items}
		},
		Row: func(string) []string { t.Fatal("JSON invoked the text formatter"); return nil },
	})
	if err != nil || out.String() != "{\"hits\":0,\"files\":[]}\n" {
		t.Fatal("envelope lost its normalized array", out.String(), err)
	}
}

type failedWriter struct{ err error }

func (w failedWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRenderListPropagatesOutputFailures(t *testing.T) {
	want := errors.New("output unavailable")
	for _, options := range []ListOptions[string]{
		{JSON: true},
		{EmptyMessage: "No entries"},
		{Headers: []string{"VALUE"}, Row: func(value string) []string { return []string{value} }},
	} {
		if err := RenderList(failedWriter{want}, []string(nil), options); !errors.Is(err, want) {
			t.Fatal("output failure discarded", err)
		}
	}
}
