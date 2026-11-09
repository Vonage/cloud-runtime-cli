package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const (
	manifestName             = "vcr.yml"
	manifestPerm os.FileMode = 0o644
)

// ReadManifest finds and parses the template's manifest among manifest.Names inside the root (newest wins) and
// returns it with its file name; ok is false when the template has none.
func (p *Project) ReadManifest() (m *manifest.Manifest, name string, ok bool, err error) {
	name, err = p.newestManifest()
	if err != nil || name == "" {
		return nil, "", false, err
	}
	data, err := p.root.ReadFile(name)
	if err == nil {
		m, err = manifest.Parse(data)
	}
	if err != nil {
		return nil, name, true, err
	}
	return m, name, true, nil
}

// newestManifest returns the name in manifest.Names whose file changed last, the earlier name of equal times, or ""
// when there is none. A path that cannot be examined, such as a link that leads out of the project, is an error.
func (p *Project) newestManifest() (string, error) {
	var newest string
	var newestTime time.Time
	for _, name := range manifest.Names {
		info, err := p.root.Stat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest, newestTime = name, info.ModTime()
		}
	}
	return newest, nil
}

// WriteManifest writes vcr.yml (0644) inside the root, replacing a template manifest named oldName when it differs
// (write then root.Rename, as v2.6.1 does; no rename when the names are equal).
func (p *Project) WriteManifest(m *manifest.Manifest, oldName string) error {
	if err := p.writeManifestFile(m, oldName); err != nil {
		return fmt.Errorf("failed to write manifest to %q: %w", filepath.Join(p.dir, oldName), err)
	}
	if oldName == manifestName {
		return nil
	}
	if err := p.root.Rename(oldName, manifestName); err != nil {
		return fmt.Errorf("failed to rename manifest file to %q : %w", filepath.Join(p.dir, manifestName), err)
	}
	return nil
}

func (p *Project) writeManifestFile(m *manifest.Manifest, name string) error {
	data, err := manifest.Marshal(m)
	if err != nil {
		return err
	}
	f, err := p.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, manifestPerm)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
