package platform_test

import (
	"vonage-cloud-runtime-cli/internal/debugsession"
	"vonage-cloud-runtime-cli/internal/deploy"
	"vonage-cloud-runtime-cli/internal/platform"
)

// The packages that consume the client import this one, so the check lives in the external test package. A
// signature that drifts away from what they call stops the build here.
var (
	_ deploy.API       = (*platform.Client)(nil)
	_ debugsession.API = (*platform.Client)(nil)
)
