package debug

import (
	"errors"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

// regionSource is where the region of the session came from.
type regionSource int

const (
	fromFlag regionSource = iota
	fromManifest
	fromConfig
)

// regionChoice is the region the session runs in, and where the choice came from.
type regionChoice struct {
	alias  string // may be empty: the lookup then lists the regions first and reports that none was given
	source regionSource
}

// chooseRegion picks the --region flag, else the region of the manifest's instance, else the default of the config.
func chooseRegion(flagRegion string, m *manifest.Manifest, configRegion string) regionChoice {
	switch {
	case flagRegion != "":
		return regionChoice{alias: flagRegion, source: fromFlag}
	case m.Instance.Region != "":
		return regionChoice{alias: m.Instance.Region, source: fromManifest}
	}
	return regionChoice{alias: configRegion, source: fromConfig}
}

// wrap words a failure to find the region as v2.6.1 did. The start-up of the old client looked up the region of the
// flag or the config, so everything it could fail with read "failed to initialize cli". Only a region of the manifest
// that does not exist was left to the later lookup, which named the deployment client. The error stays the same
// underneath.
func (r regionChoice) wrap(err error) error {
	if r.source == fromManifest && errors.Is(err, cmdenv.ErrRegionNotFound) {
		return fmt.Errorf("failed to initialize deployment client: %w", err)
	}
	return fmt.Errorf("failed to initialize cli: %w", err)
}
