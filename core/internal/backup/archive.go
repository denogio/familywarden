// Package backup creates and inspects private, router-signed household archives.
// Inspection proves integrity and signer continuity, never restore freshness.
package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/usign"
)

const (
	Format     = "familywarden-backup-v1"
	maxFile    = 1 << 20
	maxTotal   = 256 << 20
	maxFiles   = 4096
	maxArchive = maxTotal + 8*maxFile
)

var domain = []byte("familywarden-backup-v1\n")

type Entry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Format   string            `json:"format"`
	Created  int64             `json:"created"`
	Platform string            `json:"platform"`
	Versions map[string]string `json:"versions"`
	Signer   string            `json:"signer"`
	Files    []Entry           `json:"files"`
}

// Source is supplied while the native host holds exclusive writer/native locks.
// Root labels are portable archive namespaces; paths never enter the manifest.
type Source struct {
	Platform string
	Versions map[string]string
	Roots    map[string]string
	Exclude  []string // exact logical paths, including directory subtrees
	Key      usign.SecretKey
}

type input struct {
	name, file string
	size       int64
}

func validName(name string) bool {
	return name != "" && name != "." && !strings.ContainsAny(name, "\\\x00") &&
		!strings.HasPrefix(name, "/") && path.Clean(name) == name &&
		name != ".." && !strings.HasPrefix(name, "../")
}

func collect(dst string, s Source) ([]input, error) {
	var files []input
	var total int64
	seen := map[string]bool{}
	for label, root := range s.Roots {
		if !validName(label) || strings.Contains(label, "/") || label == "manifest.json" || label == "manifest.sig" {
			return nil, errors.New("invalid backup namespace")
		}
		info, err := os.Lstat(root)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("backup source root is a symlink")
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		if rel, err := filepath.Rel(root, dst); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, errors.New("backup destination overlaps a source")
		}
		err = filepath.WalkDir(root, func(file string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			name := label
			if rel != "." {
				name += "/" + filepath.ToSlash(rel)
			}
			for _, excluded := range s.Exclude {
				if name == excluded || strings.HasPrefix(name, excluded+"/") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || !validName(name) || seen[name] || info.Size() > maxFile {
				return errors.New("unsupported, duplicate or oversized backup source")
			}
			seen[name] = true
			total += info.Size()
			if total > maxTotal || len(files) >= maxFiles {
				return errors.New("backup source exceeds limits")
			}
			files = append(files, input{name, file, info.Size()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, errors.New("empty backup source")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

func header(w *tar.Writer, name string, size int64) error {
	return w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR})
}

// Create writes one no-clobber 0600 archive via a private temporary file. The
// caller must quiesce every participating writer for the entire operation.
func Create(dst string, s Source) (m Manifest, err error) {
	if len(s.Key.Key) != ed25519.PrivateKeySize || s.Platform == "" || len(s.Versions) == 0 {
		return m, errors.New("backup identity/versions missing")
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(s.Key.Key.Seed()), s.Key.Key) {
		return m, errors.New("inconsistent backup signing key")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(dst))
	if err != nil {
		return m, err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return m, err
	}
	dst = filepath.Join(parent, filepath.Base(dst))
	if _, err := os.Lstat(dst); err == nil {
		return m, errors.New("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return m, err
	}
	files, err := collect(dst, s)
	if err != nil {
		return m, err
	}
	f, err := os.CreateTemp(parent, ".familywarden-backup-*")
	if err != nil {
		return m, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	pub, _ := usign.ParsePublicKey(s.Key.Public())
	m = Manifest{Format: Format, Created: time.Now().Unix(), Platform: s.Platform, Versions: s.Versions, Signer: pub.Fingerprint(), Files: []Entry{}}
	for _, in := range files {
		if err := header(tw, in.name, in.size); err != nil {
			return m, err
		}
		entry, err := copySource(tw, in)
		if err != nil {
			return m, err
		}
		m.Files = append(m.Files, entry)
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > maxFile {
		return m, errors.New("backup manifest exceeds limits")
	}
	sig := []byte(s.Key.Sign(append(append([]byte{}, domain...), raw...)))
	for _, part := range []struct {
		name string
		data []byte
	}{{"manifest.json", raw}, {"manifest.sig", sig}} {
		if err := header(tw, part.name, int64(len(part.data))); err != nil {
			return m, err
		}
		if _, err := tw.Write(part.data); err != nil {
			return m, err
		}
	}
	if err := tw.Close(); err != nil {
		return m, err
	}
	if err := gz.Close(); err != nil {
		return m, err
	}
	if err := f.Sync(); err != nil {
		return m, err
	}
	if err := f.Close(); err != nil {
		return m, err
	}
	// Link never overwrites an existing file or symlink. Source and destination
	// are on the same filesystem; the temporary name is removed afterward.
	if err := os.Link(f.Name(), dst); err != nil {
		return m, err
	}
	d, err := os.Open(parent)
	if err != nil {
		return m, err
	}
	defer d.Close()
	return m, d.Sync()
}

func copySource(w io.Writer, in input) (Entry, error) {
	f, err := os.Open(in.file)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() != in.size {
		return Entry{}, errors.New("backup source changed")
	}
	h := sha256.New()
	if _, err := io.CopyN(io.MultiWriter(w, h), f, in.size); err != nil {
		return Entry{}, err
	}
	var extra [1]byte
	if n, err := f.Read(extra[:]); n != 0 || err != io.EOF {
		return Entry{}, errors.New("backup source grew")
	}
	after, err := f.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return Entry{}, errors.New("backup source changed")
	}
	return Entry{in.name, in.size, hex.EncodeToString(h.Sum(nil))}, nil
}

// Inspect authenticates against an externally trusted pin and verifies every
// archived byte without extracting it. A valid archive is NOT restore permission.
func Inspect(file string, trusted usign.PublicKey) (Manifest, error) {
	var m Manifest
	if len(trusted.Key) != ed25519.PublicKeySize {
		return m, errors.New("trusted backup key required")
	}
	info, err := os.Lstat(file)
	if err != nil {
		return m, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArchive {
		return m, errors.New("invalid backup file/size")
	}
	f, err := os.Open(file)
	if err != nil {
		return m, err
	}
	defer f.Close()
	compressed := bufio.NewReader(f)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return m, err
	}
	defer gz.Close()
	gz.Multistream(false)
	bounded := &io.LimitedReader{R: gz, N: maxArchive}
	tr := tar.NewReader(bounded)
	var actual []Entry
	var raw, sig []byte
	seen := map[string]bool{}
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return m, err
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Format != tar.FormatUSTAR || hdr.Linkname != "" || !validName(hdr.Name) || seen[hdr.Name] || hdr.Mode != 0600 || hdr.Size < 0 || hdr.Size > maxFile {
			return m, errors.New("invalid backup entry")
		}
		seen[hdr.Name] = true
		total += hdr.Size
		if total > maxTotal+maxFile || len(seen) > maxFiles+2 {
			return m, errors.New("backup exceeds limits")
		}
		switch hdr.Name {
		case "manifest.json":
			if sig != nil {
				return m, errors.New("backup manifest order invalid")
			}
			raw, err = io.ReadAll(tr)
		case "manifest.sig":
			if raw == nil || hdr.Size > 256 {
				return m, errors.New("backup signature order/size invalid")
			}
			sig, err = io.ReadAll(tr)
		default:
			if raw != nil {
				return m, errors.New("backup data after manifest")
			}
			h := sha256.New()
			_, err = io.Copy(h, tr)
			actual = append(actual, Entry{hdr.Name, hdr.Size, hex.EncodeToString(h.Sum(nil))})
		}
		if err != nil {
			return m, err
		}
	}
	// Consume the gzip checksum too; reject hidden trailing or concatenated data.
	var extra [1]byte
	if n, err := bounded.Read(extra[:]); n != 0 || err != io.EOF || bounded.N == 0 {
		return m, errors.New("trailing/unbounded backup data")
	}
	if _, err := compressed.Peek(1); err != io.EOF {
		return m, errors.New("trailing compressed data")
	}
	if raw == nil || sig == nil || len(actual) == 0 || !trusted.Verify(string(sig), append(append([]byte{}, domain...), raw...)) {
		return m, errors.New("backup signature invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	if m.Format != Format || m.Created <= 0 || m.Platform == "" || len(m.Versions) == 0 || m.Signer != trusted.Fingerprint() || !reflect.DeepEqual(m.Files, actual) {
		return m, fmt.Errorf("backup manifest/content mismatch")
	}
	return m, nil
}
