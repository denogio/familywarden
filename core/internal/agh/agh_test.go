package agh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// fake is a minimal in-memory AdGuard Home control API.
type fake struct {
	clients []map[string]any
	acc     access
	calls   []string
	tamper  bool // access/set stores something else than asked
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Basic dG9rZW4=" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/control/")
	f.calls = append(f.calls, r.Method+" "+path)
	var body map[string]any
	if r.Method == "POST" {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	switch path {
	case "clients":
		_ = json.NewEncoder(w).Encode(map[string]any{"clients": f.clients})
	case "access/list":
		_ = json.NewEncoder(w).Encode(f.acc)
	case "clients/add":
		f.clients = append(f.clients, body)
	case "clients/update":
		for i, c := range f.clients {
			if c["name"] == body["name"] {
				f.clients[i] = body["data"].(map[string]any)
			}
		}
	case "clients/delete":
		for i, c := range f.clients {
			if c["name"] == body["name"] {
				f.clients = append(f.clients[:i], f.clients[i+1:]...)
				break
			}
		}
	case "access/set":
		b, _ := json.Marshal(body)
		_ = json.Unmarshal(b, &f.acc)
		if f.tamper {
			f.acc.Blocked = append(f.acc.Blocked, "surprise")
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func run(t *testing.T, f *fake, in Input) Result {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	defer srv.Close()
	in.URL, in.Domain = srv.URL+"/", "dns.example.net"
	if in.Auth == "" {
		in.Auth = "dG9rZW4="
	}
	return Sync(in, srv.Client())
}

func household() *fake {
	return &fake{
		clients: []map[string]any{{"name": "Grandma tablet", "ids": []any{"grandma"}}, {"name": "Someone else", "ids": []any{"hijack"}}},
		acc:     access{Allowed: []string{"10.0.0.0/8", "grandma", "teen"}, Disallowed: []string{"192.0.2.66"}, Blocked: []string{"version.bind"}},
	}
}

func TestSlugAndClientIDs(t *testing.T) {
	for in, want := range map[string]string{"Ålex Junior": "aalex-junior", "  Øre--Ünd ": "oere-uend", "😀": "", "Émile": "mile", strings.Repeat("ab ", 30): "ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-a"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	for id, ok := range map[string]bool{"fw0a": true, "-x": false, "x-": false, "A": false, "": false, strings.Repeat("a", 64): false} {
		if ValidClientID(id) != ok {
			t.Errorf("ValidClientID(%q) != %v", id, ok)
		}
	}
}

func TestSyncAddsOwnClientsAndKeepsForeignState(t *testing.T) {
	f := household()
	res := run(t, f, Input{Profiles: []Profile{
		{ID: "alex", Name: "Ålex Junior", Protections: []string{"safe_search", "filter_lists", "safe_search"}, Services: []string{"youtube", "bad id"}},
		{ID: "gran", Name: "Grandma"},
	}})
	if !res.Complete || res.Error != "" {
		t.Fatal(res)
	}
	if !reflect.DeepEqual(res.Allocated, []Allocation{{"alex", "aalex-junior"}, {"gran", "grandma-2"}}) {
		t.Fatal(res.Allocated)
	}
	if !reflect.DeepEqual(res.Ack, []string{"alex aalex-junior dns.example.net filter_lists,safe_search youtube,bad id", "gran grandma-2 dns.example.net - -"}) {
		t.Fatal(res.Ack)
	}
	if !reflect.DeepEqual(f.acc.Allowed, []string{"10.0.0.0/8", "grandma", "teen", "aalex-junior", "grandma-2"}) || !res.WriteOwned || !reflect.DeepEqual(res.Owned, []string{"aalex-junior", "grandma-2"}) {
		t.Fatal(f.acc, res.Owned)
	}
	if !reflect.DeepEqual(f.acc.Disallowed, []string{"192.0.2.66"}) || !reflect.DeepEqual(f.acc.Blocked, []string{"version.bind"}) {
		t.Fatal(f.acc)
	}
	added := f.clients[2]
	if added["name"] != "familywarden-aalex-junior" || added["safe_search"].(map[string]any)["youtube"] != true || !reflect.DeepEqual(added["blocked_services"], []any{"youtube"}) {
		t.Fatal(added)
	}

	// Switching DNS off deletes only our client and our own allowed id.
	res = run(t, f, Input{Owned: res.Owned, Taken: []string{"aalex-junior", "grandma-2"}, Profiles: []Profile{{ID: "gran", ClientID: "grandma-2"}}})
	if !res.Complete || len(f.clients) != 3 || !reflect.DeepEqual(f.acc.Allowed, []string{"10.0.0.0/8", "grandma", "teen", "grandma-2"}) || !reflect.DeepEqual(res.Owned, []string{"grandma-2"}) {
		t.Fatal(res, f.clients, f.acc)
	}
}

func TestSyncNeverTakesOverOrWidens(t *testing.T) {
	f := household()
	old := "parent hijack dns.example.net - -"
	res := run(t, f, Input{Profiles: []Profile{{ID: "parent", ClientID: "hijack", Ack: old}, {ID: "x", ClientID: "-bad"}}})
	if res.Error != "invalid ClientID for x" || !res.Complete || !reflect.DeepEqual(res.Ack, []string{old}) || len(f.clients) != 2 {
		t.Fatal(res, f.clients)
	}

	// An open (empty) allow list stays open; the owned list is left alone.
	f = household()
	f.acc.Allowed = nil
	res = run(t, f, Input{Profiles: []Profile{{ID: "alex", ClientID: "fw01"}}})
	if !res.Complete || res.WriteOwned || len(f.acc.Allowed) != 0 {
		t.Fatal(res, f.acc)
	}

	// A list that differs after writing is reported, never acknowledged.
	f = household()
	f.tamper = true
	if res = run(t, f, Input{Profiles: []Profile{{ID: "alex", ClientID: "fw01"}}}); res.Complete || !strings.Contains(res.Error, "changed unexpectedly") {
		t.Fatal(res)
	}

	// Rejected credentials change nothing.
	f = household()
	if res = run(t, f, Input{Auth: "d3Jvbmc=", Profiles: []Profile{{ID: "alex", ClientID: "fw01"}}}); res.Complete || res.Error != "AdGuard Home unreachable or credentials rejected" || len(f.calls) != 0 {
		t.Fatal(res, f.calls)
	}
}
