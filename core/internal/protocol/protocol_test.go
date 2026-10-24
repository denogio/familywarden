package protocol

import (
	"bytes"
	"testing"
)

func TestMessageBindsTypeAndExactBytes(t *testing.T) {
	raw := []byte("{ \"minutes\": 20 }\n")
	got := Message("request", raw)
	if string(got) != "familywarden-request-v1\n"+string(raw) {
		t.Fatal("signing domain or exact bytes changed")
	}
	if bytes.Equal(got, Message("decision", raw)) {
		t.Fatal("message domains overlap")
	}
	if bytes.Equal(got, Message("request", []byte("{\"minutes\":20}"))) {
		t.Fatal("JSON was canonicalized")
	}
}
