package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCGIAppIsAnExactBoundedPOSTForwarder(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "core.sock")
	l, err := listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || r.URL.Path != "/app/request" || r.Method != "POST" || string(body) != "signature\nexact JSON" {
			t.Error("forwarding changed the request", r.URL.Path, string(body), err)
		}
		_, _ = w.Write([]byte("router signature\nexact reply"))
	}))
	srv.Listener = l
	srv.Start()
	defer srv.Close()
	for _, query := range []string{"request", "decision", "op=grant", "op=request&op=decision", "op=request&ignored=1", ""} {
		t.Setenv("QUERY_STRING", query)
		t.Setenv("REQUEST_METHOD", "POST")
		var out bytes.Buffer
		cgiApp(socket, strings.NewReader("signature\nexact JSON"), &out)
		if !strings.HasPrefix(out.String(), "Status: 400 ") {
			t.Fatal("invalid query forwarded", query, out.String())
		}
	}
	t.Setenv("QUERY_STRING", "op=request")
	t.Setenv("REQUEST_METHOD", "GET")
	var out bytes.Buffer
	cgiApp(socket, strings.NewReader("signature\nexact JSON"), &out)
	if calls.Load() != 0 || !strings.HasPrefix(out.String(), "Status: 400 ") {
		t.Fatal("invalid transport reached the core")
	}
	t.Setenv("REQUEST_METHOD", "POST")
	out.Reset()
	cgiApp(socket, strings.NewReader(strings.Repeat("x", maxAPI+1)), &out)
	if calls.Load() != 0 || !strings.HasPrefix(out.String(), "Status: 400 ") {
		t.Fatal("oversized upload forwarded")
	}
	out.Reset()
	cgiApp(socket, strings.NewReader("signature\nexact JSON"), &out)
	if calls.Load() != 1 || !strings.HasSuffix(out.String(), "\n\nrouter signature\nexact reply") {
		t.Fatal("reply was not forwarded exactly", out.String())
	}
	srv.Close()
	out.Reset()
	cgiApp(socket, strings.NewReader("signature\nexact JSON"), &out)
	if !strings.HasSuffix(out.String(), "\n\n") || !strings.HasPrefix(out.String(), "Status: 503 ") {
		t.Fatal("core outage produced authority", out.String())
	}
}

func TestAPIShimDropsOnlyTheLuCISession(t *testing.T) {
	in := `{"request_id":"r-00000001","approve":true,"minutes":30,"ubus_rpc_session":"895e0d5bf88a61c99762d778a4b7682b"}`
	got := string(withoutSession([]byte(in)))
	if strings.Contains(got, "ubus_rpc_session") || !strings.Contains(got, `"approve":true`) || !strings.Contains(got, `"minutes":30`) {
		t.Fatalf("session not stripped or fields lost: %s", got)
	}
	for _, raw := range []string{
		`{"request_id":"r-00000001","approve":true}`, `not json`, `[]`, ``,
		`{"approve":false,"approve":true,"ubus_rpc_session":"s"}`,
		`{"approve":false,"\u0061pprove":true,"ubus_rpc_session":"s"}`,
		`{"approve":true,"ubus_rpc_session":"a","ubus_rpc_session":"b"}`,
		`{"approve":true,"ubus_rpc_session":"s"} {}`,
		`{"ubus_rpc_session":"` + strings.Repeat("x", maxAPI) + `"}`,
		"{\"ubus_rpc_session\":\"\xff\"}",
	} {
		if string(withoutSession([]byte(raw))) != raw {
			t.Fatalf("changed input without a session: %q", raw)
		}
	}
}
