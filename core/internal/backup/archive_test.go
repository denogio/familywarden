package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/denogio/familywarden/core/internal/usign"
)

func key(t *testing.T) usign.SecretKey {
	t.Helper()
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return usign.SecretKey{KeyID: [8]byte{1}, Key: k}
}

func fixture(t *testing.T) (Source, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "core"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"core/configuration.json": `{"version":1}`, "mast.key": "fictional-secret", "core/.writer.lock": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return Source{Platform: "test-platform", Versions: map[string]string{"core": "test", "adapter": "fixture"}, Roots: map[string]string{"household": root}, Exclude: []string{"household/core/.writer.lock"}, Key: key(t)}, filepath.Join(t.TempDir(), "household.tar.gz")
}

func TestArchivePrivateSignedRoundTripAndNoClobber(t *testing.T) {
	s, out := fixture(t)
	want, err := Create(out, s)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := usign.ParsePublicKey(s.Key.Public())
	got, err := Inspect(out, pub)
	if err != nil || !reflect.DeepEqual(got, want) || len(got.Files) != 2 {
		t.Fatal(got, err)
	}
	stat, err := os.Stat(out)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal(stat, err)
	}
	before, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(out, s); err == nil {
		t.Fatal("existing backup overwritten")
	}
	after, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("no-clobber changed existing data", err)
	}
	other, _ := usign.ParsePublicKey(key(t).Public())
	if _, err := Inspect(out, other); err == nil {
		t.Fatal("foreign router pin accepted")
	}
	if _, err := Inspect(out, usign.PublicKey{}); err == nil {
		t.Fatal("missing trusted pin accepted")
	}
}

func TestSourcesRefuseLinksSpecialFilesOversizeAndOutputOverlap(t *testing.T) {
	for _, kind := range []string{"symlink", "directory-symlink", "oversize", "overlap", "alias-overlap"} {
		t.Run(kind, func(t *testing.T) {
			s, out := fixture(t)
			root := s.Roots["household"]
			switch kind {
			case "symlink":
				if err := os.Symlink("mast.key", filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				s.Roots["household"] = alias
			case "oversize":
				f, err := os.Create(filepath.Join(root, "huge"))
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(maxFile + 1)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			case "overlap":
				out = filepath.Join(root, "archive.tar.gz")
			case "alias-overlap":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				out = filepath.Join(alias, "archive.tar.gz")
			}
			if _, err := Create(out, s); err == nil {
				t.Fatal("unsafe source/destination accepted")
			}
			if _, err := os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("failed backup was published", err)
			}
		})
	}
}

// Rewrite the artifact, not source state. The signature must still bind the
// exact manifest and hashes; archive structure is validated independently.
func rewrite(t *testing.T, in, out string, change func(*tar.Header, []byte) []byte) {
	t.Helper()
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	tw := tar.NewWriter(z)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		data = change(h, data)
		h.Size = int64(len(data))
		if h.Typeflag != tar.TypeReg {
			h.Size = 0
			data = nil
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectionRefusesTamperingTraversalLinksAndTrailingData(t *testing.T) {
	s, original := fixture(t)
	if _, err := Create(original, s); err != nil {
		t.Fatal(err)
	}
	pub, _ := usign.ParsePublicKey(s.Key.Public())
	for _, kind := range []string{"content", "manifest", "signature", "traversal", "duplicate", "symlink", "oversize", "truncated", "concatenated", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "bad.tar.gz")
			if kind == "truncated" || kind == "concatenated" || kind == "trailing" {
				data, err := os.ReadFile(original)
				if err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "truncated":
					data = data[:len(data)-4]
				case "concatenated":
					data = append(data, data...)
				case "trailing":
					data = append(data, []byte("hidden")...)
				}
				if err := os.WriteFile(out, data, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				rewrite(t, original, out, func(h *tar.Header, data []byte) []byte {
					switch kind {
					case "content":
						if h.Name == "household/mast.key" {
							return []byte("changed-secret")
						}
					case "manifest":
						if h.Name == "manifest.json" {
							return bytes.Replace(data, []byte("test-platform"), []byte("other-platform"), 1)
						}
					case "signature":
						if h.Name == "manifest.sig" {
							return []byte("invalid")
						}
					case "traversal":
						if h.Name == "household/mast.key" {
							h.Name = "../escaped"
						}
					case "duplicate":
						if h.Name == "household/mast.key" {
							h.Name = "household/core/configuration.json"
						}
					case "symlink":
						if h.Name == "household/mast.key" {
							h.Typeflag = tar.TypeSymlink
							h.Linkname = "../../outside"
						}
					case "oversize":
						if h.Name == "household/mast.key" {
							return make([]byte, maxFile+1)
						}
					}
					return data
				})
			}
			if _, err := Inspect(out, pub); err == nil {
				t.Fatal("invalid backup accepted")
			}
		})
	}
}
