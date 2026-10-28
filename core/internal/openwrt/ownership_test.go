package openwrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/denogio/familywarden/core/internal/configuration"
)

func TestNativeOwnershipRoutingNeverTreatsDamageAsLegacy(t *testing.T) {
	for _, name := range []string{"configuration.json", configuration.OwnershipDocument, "configuration.json.present", "authority.json.present"} {
		for _, kind := range []string{"regular", "dangling", "directory"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				a := Adapter{ConfigFile: filepath.Join(dir, "configuration.json")}
				if owned, err := a.configurationOwned(); err != nil || owned {
					t.Fatal("empty native household is not legacy", owned, err)
				}
				path := filepath.Join(dir, name)
				var err error
				switch kind {
				case "regular":
					err = os.WriteFile(path, []byte("not JSON"), 0600)
				case "dangling":
					err = os.Symlink(filepath.Join(dir, "missing"), path)
				default:
					err = os.Mkdir(path, 0700)
				}
				if err != nil {
					t.Fatal(err)
				}
				if owned, err := a.configurationOwned(); err != nil || !owned {
					t.Fatal("damaged ownership selected legacy routing", owned, err)
				}
			})
		}
	}
}
