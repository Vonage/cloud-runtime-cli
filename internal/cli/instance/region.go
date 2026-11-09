package instance

import (
	"context"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

// regionClient returns the client of the region inst runs in. The resolver fetches a region record once per
// alias, so the configured region is not looked up again. An instance row without a region alias is reached
// through the configured region.
func regionClient(ctx context.Context, env *cmdenv.Env, inst platform.Instance) (*platform.Client, error) {
	if inst.RegionAlias == "" {
		return env.Client, nil
	}
	client, err := env.Resolver.Client(ctx, inst.RegionAlias)
	if err != nil {
		return nil, fmt.Errorf("failed to reach region %q: %w", inst.RegionAlias, err)
	}
	return client, nil
}
