package cmdenv

import (
	"context"
	"errors"

	"vonage-cloud-runtime-cli/internal/platform"
)

// RegionClient resolves alias the way v2.6.1's start-up did. An empty alias first calls Resolver.Regions, so bad
// credentials still fail with the platform's error, and then fails with ErrRegionNotFound; an unknown alias fails
// with ErrRegionNotFound; other errors are returned unchanged. Callers wrap the error as
// "failed to initialize cli: %w". Used by the root's pre-run for full commands, and by deploy and debug.
func (e *Env) RegionClient(ctx context.Context, alias string) (*platform.Client, error) {
	if alias == "" {
		if _, err := e.Resolver.Regions(ctx); err != nil {
			return nil, err
		}
		return nil, ErrRegionNotFound
	}
	client, err := e.Resolver.Client(ctx, alias)
	if errors.Is(err, platform.ErrNotFound) {
		return nil, ErrRegionNotFound
	}
	if err != nil {
		return nil, err
	}
	return client, nil
}
