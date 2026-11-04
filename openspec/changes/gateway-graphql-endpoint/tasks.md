# Tasks

## 1. Gateway endpoints

- [ ] 1.1 In `pkg/cmdutil/factory_test.go`, change `TestMakeGraphqlEndpoint` to expect the gateway URL for `aws.euw1`, `aws.use1`, `aws.apse1` and `aws.apse2`, and an empty result for `aws.euc1`, `aws.usw2`, `aws.mars1` and the malformed cases; add an `IsValidRegion` table test. Run `go test ./pkg/cmdutil/...` and see it fail.
- [ ] 1.2 In `pkg/cmdutil/factory.go`, add the region→geography table, make `IsValidRegion` check membership and `makeGraphqlEndpoint` build the gateway URL. Verify: `go test ./pkg/cmdutil/...` passes.
- [ ] 1.3 In `vcr/root/root.go`, make the invalid `--region` error list the supported regions; cover it in the root command tests. Verify: `go test ./vcr/root/...`.
- [ ] 1.4 In `pkg/config/config.go`, set `DefaultGraphqlURL` to the EU gateway URL; update any test that asserts the old value. Verify: `go test ./pkg/config/... ./vcr/configure/...`.

## 2. Release tooling

- [ ] 2.1 In `.github/workflows/release-please.yml` and `.github/workflows/release.yml`, run "Store notarization credentials", "Zip darwin binaries for notarization" and "Notarize darwin binaries" only when `vars.APPLE_NOTARIZE == 'true'`; update the build job comments. Verify: `actionlint` (if installed) or a YAML parse of both files.

## 3. Checks

- [ ] 3.1 Run `make test` and `golangci-lint run` (v2.6.2); both pass.
