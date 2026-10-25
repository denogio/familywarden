package identity

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
)

func TestProfileDirectoryReturnsAllDisplayIdentitiesWithoutPolicyData(t *testing.T) {
	s, ad, _ := setup(t)
	ad.ctx.Profiles = map[string]platform.Profile{
		"sam":   {Name: "Sam", AppsRev: "private policy revision", Requires: []string{"apps-v1"}},
		"alex":  {Name: "Alex"}, // app control off; still a valid enrollment target
		"blank": {},
	}
	want := []Reply{{"id": "alex", "name": "Alex"}, {"id": "blank", "name": "blank"}, {"id": "sam", "name": "Sam"}}
	got := s.ProfileDirectory()
	if !reflect.DeepEqual(got["profiles"], want) {
		t.Fatal(got)
	}
	b, err := json.Marshal(got)
	if err != nil || strings.Contains(string(b), "apps") || strings.Contains(string(b), "private") {
		t.Fatal("profile selector leaked policy content", string(b), err)
	}
	ad.ctx.Profiles = nil
	b, _ = json.Marshal(s.ProfileDirectory())
	if string(b) != `{"profiles":[]}` {
		t.Fatal("empty directory must be an array", string(b))
	}
	ad.ctxErr = errors.New("adapter unavailable")
	if got := s.ProfileDirectory(); got["error"] != "platform state unavailable" || got["profiles"] != nil {
		t.Fatal("failure must not masquerade as an empty registry", got)
	}
}
