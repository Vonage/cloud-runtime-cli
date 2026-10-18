package configure

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/platform"
)

// runRich is run for a terminal. It reads the file, asks the questions and writes the file as the classic run does,
// with the same warning, the same prompts and the same errors; the two steps that wait show a live line and leave a
// result line.
func runRich(ctx context.Context, env *cmdenv.Env) error {
	path := env.Flags.ConfigFile // as given: the messages show it the way the user typed it
	stored := readStored(env, path)
	cur := currentOf(env.Flags, stored)

	key, secret, err := askCredentials(env.Prompt, cur)
	if err != nil {
		return err
	}
	regions, err := fetchRegionsRich(ctx, env, stored, platform.Credentials{APIKey: key, APISecret: secret})
	if err != nil {
		return err
	}
	region, err := chooseRegion(env.Prompt, regions, cur.region)
	if err != nil {
		return err
	}

	step := env.IO.StartStep("Writing configuration")
	err = config.Write(path, fileToWrite(env.Flags, stored, key, secret, region))
	if err != nil {
		step.Stop()
		return fmt.Errorf("failed to write config: %w", err)
	}
	step.OK("New configuration file written to " + path)
	return nil
}

func fetchRegionsRich(ctx context.Context, env *cmdenv.Env, stored config.File, creds platform.Credentials) ([]platform.Region, error) {
	resolver, err := newResolver(env, stored, creds)
	if err != nil {
		return nil, err
	}

	step := env.IO.StartStep("Retrieving regions")
	regions, err := resolver.Regions(ctx)
	if err != nil {
		step.Stop()
		return nil, fmt.Errorf("failed to retrieve regions: %w", err)
	}
	step.OK("Regions retrieved")
	return regions, nil
}
