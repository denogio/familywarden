package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestStatelessAppCompilerIsBoundedAndProducesNoPartialSnapshot(t *testing.T) {
	for _, raw := range []string{
		``, `{}`, `null`, `{"version":2}`, `{"version":1,"unknown":true}`,
		`{"version":1} {}`, `{"version":1,"apps":42}`,
		`{"version":1,"enabled":"` + strings.Repeat("x", 1<<20) + `"}`,
		"{\"version\":1,\"enabled\":\"\xff\"}",
	} {
		var out bytes.Buffer
		if err := appCanon(strings.NewReader(raw), &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid input made a snapshot", raw[:min(60, len(raw))], err, out.String())
		}
	}
	var out bytes.Buffer
	if err := appCanon(strings.NewReader(`{"version":1,"enabled":"0"}`), &out); err != nil || out.Len() != 0 {
		t.Fatal(err, out.String())
	}
	raw := `{"version":1,"enabled":"1","apps":[{"package":"com.example.video","minutes":"20"}]}`
	if err := appCanon(strings.NewReader(raw), &out); err != nil || out.String() != "app|com.example.video|20|0\nnew|1||\n" {
		t.Fatal(err, out.String())
	}
	if err := appCanon(strings.NewReader(raw), brokenOutput{}); err == nil {
		t.Fatal("snapshot output failure hidden")
	}
}
