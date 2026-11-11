# Tasks

`<WT>` is this worktree. Line numbers are `origin/main` at planning time (release 3.0.0). Tests use fakes and
`httptest`, never mock generators (AGENTS.md). Every task works test first: write the test, run it and watch it fail
for the right reason, then write the code. Every task ends with `cd <WT> && go test ./<package>/...` passing for the
packages it touched, and every group ends with `cd <WT> && make test` passing before its commit.

## Plan notes

- **Debug check placement.** The design puts the debug checks in `preflight`, which runs after `RegionClient` and the
  application picker, both platform requests; the spec says "before any platform call". The plan calls the check once
  in `execute` after `loadProject` (task 4.2) and not in `preflight`. Deploy likewise checks right after `manifest.Read`,
  before the region lookup and so before `stages.Run`.
- **Walker home.** `WebhookAddresses` lives in `internal/platform`, which both consumers already import.
- **Non-text keys.** yaml.v3 decodes a nested mapping with a non-text key to `map[any]any`, which `encoding/json` cannot
  send. `CheckApplicationCapabilities` rejects it up front (small addition; drop that case in 1.3 if unwanted).
- **Label.** The label path is that of the object holding `address` (`voice.webhooks.answer_url`), as in the spec example.
- **Absent response block.** JSON `null` counts as absent; `{}` counts as applied.
- **Intermediate commits.** Group 2 changes `DeployDebugService` and passes `nil`; group 4 passes the block.

## Exact strings

Defined once here; the tasks refer to them by name.

- `ErrBlockAndList` (manifest): `application-capabilities and instance.capabilities cannot both be set: remove one of them`
- `ErrBlockAndFlag` (manifest): `application-capabilities and --capabilities cannot both be set: remove one of them`
- Unknown token (manifest, formatted): `application-capabilities uses unknown tokens: {{a}}, {{b}}; the only valid token is {{instance_url}}`
  (tokens sorted, each once, joined with `, `).
- Non-text key (manifest, formatted): `application-capabilities keys must be text, found 200`
- `errInstanceApplication` (cli/debug): `application-capabilities would point the webhooks of the instance application at this debug session: set debug.application-id or --app-id to a different application`
- `platform.NotAppliedNotice`: `application-capabilities was not applied: this region's platform did not report applying it`.
  Printed on stderr as `<warning icon> <notice>\n` (`! ` in the test streams).
- Webhook label: `Webhook <dotted path>` (`platform.WebhookAddress.Label()`).
  - Classic deploy and debug intro line: `| Webhook <path>: <address>` (the address through `term.YellowBold`, like the host line).
  - Rich rows: `term.Row{Label: "Webhook <path>", Value: <address>}`.
  - Never `Instance host address`: the contract's scraped pattern `^(\| )?Instance host address: \S+$` must see no new line.

## 1. Manifest

- [x] 1.1 Failing tests in new `internal/manifest/appcaps_test.go` (package `manifest_test`, helpers in `helpers_test.go`).
  - Helper `exampleBlock() map[string]any` builds the spec block as plain maps:
    `{"voice": {"webhooks": {"answer_url": {"address": "{{instance_url}}/answer", "http_method": "POST"}}, "signed_callbacks": true}}`.
  - `TestParseReadsTheBlockAsWritten`: `manifest.Parse` of
    `application-capabilities: {voice: {webhooks: {answer_url: {address: "{{instance_url}}/answer", http_method: POST}}, signed_callbacks: true}}`
    gives `ApplicationCapabilities` equal to `exampleBlock()`, and `json.Marshal` of it is exactly
    `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":{"address":"{{instance_url}}/answer","http_method":"POST"}}}}`.
    A second row keeps `rtc: null` (`{"rtc":null}` marshals as written) and an array (`messages: {numbers: [a, b]}`).
  - `TestAbsentOrEmptyBlockIsNone`: inputs `""` (no key), `application-capabilities: {}`, `application-capabilities:` (null)
    all give `len(m.ApplicationCapabilities) == 0`.
  - `TestMarshalKeepsTheBlock`: `Marshal` then `Parse` of a manifest with `exampleBlock()` equals the original (compare
    parsed values, never the YAML bytes: keys come out sorted and the address is single-quoted).
  - `TestMergeKeepsAndDeepCopiesTheBlock`: `base := fullManifest()` with `ApplicationCapabilities = exampleBlock()`;
    `got := manifest.Merge(base, answers())` carries an equal block; then write through the copy (nested
    `signed_callbacks = false`, `delete` of `webhooks`, append to a nested slice) and assert `base` still equals
    `exampleBlock()`. Use checked assertions (`v, ok := x.(map[string]any)`; `require.True(t, ok)`) because
    `forcetypeassert` is enabled.
  - Do not change `fullManifest()`, `testdata/full.yml` or `defaults.yml`: the existing `TestMarshalGolden` and
    `TestReadParsesEverySection` are the proof that a manifest without a block marshals byte for byte as before.
  - Verify: `cd <WT> && go test ./internal/manifest/` fails to compile (no field yet).
- [x] 1.2 Add the field and the deep clone.
  - `internal/manifest/manifest.go:8-12`, after `Debug`:
    ```go
    // ApplicationCapabilities is the Vonage application "capabilities" object exactly as written in vcr.yml. The
    // CLI sends it as is; only CheckApplicationCapabilities looks inside.
    ApplicationCapabilities map[string]any `yaml:"application-capabilities,omitempty"`
    ```
  - `internal/manifest/merge.go:24-38` `cloneManifest`: add
    `clone.ApplicationCapabilities = cloneBlock(m.ApplicationCapabilities)`. New `internal/manifest/appcaps.go`:
    ```go
    func cloneBlock(b map[string]any) map[string]any {   // nil stays nil; {} stays {}
        if b == nil { return nil }
        out := make(map[string]any, len(b))
        for k, v := range b { out[k] = cloneValue(v) }
        return out
    }
    func cloneValue(v any) any { // maps and slices are copied, scalars are immutable
        switch v := v.(type) {
        case map[string]any: return cloneBlock(v)
        case map[any]any:    out := make(map[any]any, len(v)); for k, e := range v { out[k] = cloneValue(e) }; return out
        case []any:          out := make([]any, len(v)); for i, e := range v { out[i] = cloneValue(e) }; return out
        }
        return v
    }
    ```
  - `Manifest.Merge` keeps copying only the six values; the block comes from `base`, which is what `vcr init` needs.
  - Verify: 1.1 passes; `TestMarshalGolden` and `TestMergeSharesNothingWithTheBase` still pass.
- [x] 1.3 Failing table test `TestCheckApplicationCapabilities` in `appcaps_test.go`, then
  `CheckApplicationCapabilities` in `internal/manifest/appcaps.go`.
  - Signature and sketch:
    ```go
    const InstanceURLToken = "{{instance_url}}"

    var (
        ErrBlockAndList = errors.New("application-capabilities and instance.capabilities cannot both be set: remove one of them")
        ErrBlockAndFlag = errors.New("application-capabilities and --capabilities cannot both be set: remove one of them")
        tokenPattern    = regexp.MustCompile(`\{\{.*?\}\}`)
    )

    // flagCapabilities is the already split --capabilities list (nil when the flag is empty).
    func CheckApplicationCapabilities(m *Manifest, flagCapabilities []string) error {
        if len(m.ApplicationCapabilities) == 0 { return nil }            // no block: nothing to check, ever
        if len(m.Instance.Capabilities) > 0 { return ErrBlockAndList }   // the manifest pair is reported first
        if len(flagCapabilities) > 0 { return ErrBlockAndFlag }
        unknown := map[string]struct{}{}
        if err := scanTokens(m.ApplicationCapabilities, unknown); err != nil { return err }
        if len(unknown) == 0 { return nil }
        return fmt.Errorf("application-capabilities uses unknown tokens: %s; the only valid token is %s",
            strings.Join(slices.Sorted(maps.Keys(unknown)), ", "), InstanceURLToken)
    }
    ```
    `scanTokens(v any, unknown map[string]struct{}) error` walks `map[string]any` values, `map[any]any` values,
    `[]any` elements, keys and strings (the platform substitutes in the serialised block and refuses an unknown token
    anywhere in it, keys included): for a string or a key, every `tokenPattern` match other than
    `InstanceURLToken` goes into `unknown`. A `map[any]any` with a key that is not a `string` returns
    `fmt.Errorf("application-capabilities keys must be text, found %v", k)` (when several, report the smallest by
    `fmt.Sprint`, so the text is stable).
  - Cases (`name: manifest, flag -> want`); build the manifest with direct struct literals:

    | name | block | `Instance.Capabilities` | flag | want |
    |---|---|---|---|---|
    | no block, list and flag | none | `["voice"]` | `["voice"]` | nil |
    | empty block `{}` | `{}` | `["voice"]` | none | nil |
    | block alone | `exampleBlock()` | none | none | nil |
    | block and instance.capabilities | `exampleBlock()` | `["voice"]` | none | `ErrBlockAndList` |
    | block and flag | `exampleBlock()` | none | `["voice"]` | `ErrBlockAndFlag` |
    | block, list and flag | `exampleBlock()` | `["voice"]` | `["rtc"]` | `ErrBlockAndList` |
    | empty (non-nil) list and flag | `exampleBlock()` | `[]string{}` | `[]string{}` | nil |
    | unknown token | address `{{instance_host}}/answer` | none | none | `application-capabilities uses unknown tokens: {{instance_host}}; the only valid token is {{instance_url}}` |
    | unknown tokens, nested in a map and an array, sorted, one repeated | `voice.webhooks.a.address: "{{b}}"`, `messages.numbers: ["x", "{{a}}", "{{b}}"]` | none | none | `... unknown tokens: {{a}}, {{b}}; ...` |
    | valid and unknown in one string | address `{{instance_url}}/x/{{id}}` | none | none | `... unknown tokens: {{id}}; ...` |
    | valid token twice | two addresses `{{instance_url}}/a`, `{{instance_url}}/b` | none | none | nil |
    | literal address | address `https://example.com/answer` | none | none | nil |
    | spaces make a different token | address `{{ instance_url }}/a` | none | none | `... unknown tokens: {{ instance_url }}; ...` |
    | unterminated is not a token | address `{{instance_url` | none | none | nil |
    | token in a key is checked too | key `{{x}}` with value `"y"` | none | none | `... unknown tokens: {{x}}; ...` |
    | non-string values | `{"a": true, "b": 5, "c": nil, "d": 1.5}` | none | none | nil |
    | non-text key | `{"voice": map[any]any{200: "x"}}` | none | none | `application-capabilities keys must be text, found 200` |

    Also `TestCheckApplicationCapabilitiesFromYAML`: `manifest.Parse` of `application-capabilities: {voice: {webhooks: {200: {address: x}}}}`
    then `CheckApplicationCapabilities` returns the non-text key error (proves the real parse produces `map[any]any`).
  - The error texts are pinned here as literals with `assert.EqualError`; other packages use `require.ErrorIs`
    against the exported vars, or `ErrorContains`.
  - Verify: `cd <WT> && go test ./internal/manifest/ -run 'CheckApplicationCapabilities|Block'`.
- [x] 1.4 `vcr init` keeps the block. Failing test first in `internal/cli/initcmd/template_test.go` (file is 211 lines; the new
  test is short):
  `TestTemplateManifestKeepsTheApplicationCapabilitiesBlock`. Use `templateHarness(t, tarGz(t, entry("index.js", "x\n"),
  entry("vcr.yml", templateManifest+blockYAML)))` where `blockYAML` is
  `application-capabilities: {voice: {webhooks: {answer_url: {address: "{{instance_url}}/answer", http_method: POST}}, signed_callbacks: true}}\n`
  (append it after `unknown-key: dropped\n`, as a top-level key). `require.NoError(t, h.run())`,
  `got, err := manifest.Read(h.path("vcr.yml"))`, assert `got.ApplicationCapabilities` equals the same nested maps
  (`map[string]any{"voice": map[string]any{"signed_callbacks": true, "webhooks": ...}}`), `assert.NotContains` of
  `unknown-key` still holds, and `h.stdout.String()` is the usual `"✓ <path> created\n"`. A second case,
  `TestTemplateManifestWithoutABlockWritesNone`, reuses the existing `TestTemplateIsMergedOverTheAnswers` assertions and
  adds `assert.NotContains(t, readText(t, h.path("vcr.yml")), "application-capabilities")`. No production change beyond 1.2:
  `layOverTemplateManifest` (`template.go:109-122`) already goes through `manifest.Merge`. The contract's `init-*` scenarios
  have no block and compare written files, so they must stay green.
  - Verify: `cd <WT> && go test ./internal/cli/initcmd/ ./internal/scaffold/` (fails before 1.2, passes after).
- [x] 1.5 Commit: `feat: read and check an application-capabilities block in vcr.yml`.
  Before it: `cd <WT> && make test`.

## 2. Platform client

Wire contract used here: request field `applicationCapabilities` on `POST /packages` and `POST /debug/services`;
optional response object `applicationCapabilities` on `POST /deployments` and `POST /debug/services`, the block as
applied with `{{instance_url}}` replaced.

- [x] 2.1 Failing tests in new `internal/platform/appcaps_test.go` (package `platform`; helpers `newRouteServer`,
  `newRouteServerAt`, `answerJSON`, `mustJSON`, `assertCredentialsSent` from `helpers_test.go` / `routes_helpers_test.go`).
  - `TestCreatePackageSendsTheBlockOnEveryVersion`: block =
    `map[string]any{"voice": map[string]any{"signed_callbacks": true, "webhooks": map[string]any{"answer_url": map[string]any{"address": "{{instance_url}}/answer", "http_method": "POST"}}}}`;
    request `PackageRequest{SourceCodeKey: "k1", Runtime: "nodejs18", Name: "shop-dev", ApplicationCapabilities: block}`;
    `const wantBlock = `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":{"address":"{{instance_url}}/answer","http_method":"POST"}}}}``.

    | version | wantBody (exact string, `req.Body`) |
    |---|---|
    | v0.3 | `{"sourceCodeKey":"k1","entrypoint":null,"capabilities":{},"buildScriptPath":"","runtime":"nodejs18","applicationCapabilities":` + wantBlock + `}` |
    | v0.4 | same as v0.3 |
    | v0.5 | `{"sourceCodeKey":"k1","entrypoint":null,"capabilities":{},"buildScriptPath":"","runtime":"nodejs18","name":"shop-dev","applicationCapabilities":` + wantBlock + `}` |

    The field is last, so key order after the old bytes is fixed, and there is no version gate (design decision 4).
  - `TestCreatePackageWithoutABlockKeepsItsBytes`: rows `ApplicationCapabilities: nil` and `map[string]any{}` both give
    `{"sourceCodeKey":"k3","entrypoint":null,"capabilities":{},"buildScriptPath":"","runtime":"nodejs18"}` (v0.3). The existing
    `TestCreatePackageBodyAndID` and `TestCreatePackageSendsTheNameFromV05` stay as they are and keep proving the old bytes.
  - `TestDeployDebugServiceSendsTheBlock` (reply `debugServiceReply("dbg-svc-1", "inst-1")`): with `name == ""`, body
    `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","capabilities":{},"applicationCapabilities":` + wantBlock + `}`;
    with `name == "dbg"`, `...,"apiApplicationId":"app-1","name":"dbg","capabilities":{},"applicationCapabilities":...}`.
    The existing `TestDeployDebugServiceBody` and `...NoCapabilitiesIsAnEmptyObject` keep their expected strings (they
    pass `nil` for the new argument).
  - `TestDeployInstanceReturnsTheAppliedBlock`: reply
    `{"instanceId":"i-1","serviceName":"svc","deploymentId":"d-1","hostUrls":["https://h.example"],"applicationCapabilities":{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"}}}}}`;
    `assert.JSONEq` of `string(got.ApplicationCapabilities)` with the object. Rows: reply without the key
    (`got.ApplicationCapabilities` is empty and `Applied` false), reply with `"applicationCapabilities":null` (`Applied` false).
  - `TestDeployDebugServiceReturnsTheAppliedBlock`: same two rows through `DebugService`; the legacy row keeps
    `assert.Equal(t, DebugService{ServiceName: "dbg-svc-1", PrivateKey: "pk", InstanceID: "inst-1", WebsocketPath: "abc123"}, got)`
    true (the new field is nil).
  - `TestWebhookAddresses` (table, `json.RawMessage(input) -> []WebhookAddress`):

    | name | input | want |
    |---|---|---|
    | the spec example | `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}` | `{voice.webhooks.answer_url, https://h.example/answer}`, `{voice.webhooks.event_url, https://h.example/event}` |
    | input order does not matter | the same with `event_url` first | the same two, answer first |
    | other fields ignored | `{"voice":{"webhooks":{"answer_url":{"address":"https://h/a","http_method":"POST"}},"signed_callbacks":true},"rtc":null,"messages":{"version":"v1"}}` | `{voice.webhooks.answer_url, https://h/a}` |
    | non-string address ignored | `{"voice":{"webhooks":{"answer_url":{"address":5}}}}` | nil |
    | other name ignored | `{"voice":{"Address":"x","addr":"y"}}` | nil |
    | sorted as dotted strings | `{"a":{"x":{"address":"2"}},"a-b":{"address":"1"}}` | `{a-b, 1}` then `{a.x, 2}` (`-` sorts before `.`, so this is not depth-first order) |
    | arrays | `{"messages":{"webhooks":[{"address":"u"}]}}` | `{messages.webhooks.0, u}` |
    | top-level address | `{"address":"u"}` | `{address, u}` |
    | not an object | `[]`, `"x"`, `null`, `` (empty), `{` (invalid) | nil |
  - `TestApplied` (`json.RawMessage -> bool`): `nil`, `""`, `null`, ` null ` are false; `{}`, `{"voice":{}}` are true.
  - `TestNotAppliedNoticeText`: pins the literal `application-capabilities was not applied: this region's platform did not report applying it`.
  - `TestWebhookLabelIsSanitised`: `WebhookAddress{Path: "voice.\x1b[31mx"}.Label()` is `Webhook voice.x`.
  - Verify: `cd <WT> && go test ./internal/platform/` fails to compile.
- [x] 2.2 Request and response types.
  - `internal/platform/deployments.go:32-39` `PackageRequest`: append, after `Name`,
    `ApplicationCapabilities map[string]any \`json:"applicationCapabilities,omitempty"\``. `CreatePackage` (:43-57)
    is untouched: no `semver` gate, so the field goes out on every version.
  - `deployments.go:101-106` `DeployResult`: add
    `ApplicationCapabilities json.RawMessage \`json:"applicationCapabilities,omitempty"\`` (import `encoding/json`).
  - `internal/platform/debugservices.go:14-19` `DebugService`: add
    `ApplicationCapabilities json.RawMessage \`json:"applicationCapabilities,omitempty"\``.
  - `debugservices.go:22-28` `debugServiceBody`: append, after `Capabilities`,
    `ApplicationCapabilities map[string]any \`json:"applicationCapabilities,omitempty"\``. Update the comment on :30 to list the new key.
  - `debugservices.go:32` new signature (the last argument is new; `nil` and `{}` send nothing):
    ```go
    func (c *Client) DeployDebugService(ctx context.Context, region, appID, name string, caps manifest.Capabilities, appCaps map[string]any) (DebugService, error) {
        body := debugServiceBody{Runtime: debugProxyRuntime, Region: region, APIApplicationID: appID, Name: name, Capabilities: caps, ApplicationCapabilities: appCaps}
    ```
  - Update every existing call (add `, nil`): `internal/platform/debugservices_test.go:41,59,83,95,105,114`,
    `internal/platform/deployments_test.go:319`, `internal/platform/routes_fake_test.go:129`. Line :41 passes
    `nil` here; the new test in 2.1 passes the block.
- [x] 2.3 New `internal/platform/appcaps.go` (stays far below 400 lines):
  ```go
  // NotAppliedNotice is what a command warns when it sent a block and the response has none.
  const NotAppliedNotice = "application-capabilities was not applied: this region's platform did not report applying it"

  // WebhookAddress is one string field named "address" of an applied block, with the dotted path of the object that holds it.
  type WebhookAddress struct{ Path, Address string }

  // Label is the text before the colon of a summary line. The path is platform text.
  func (a WebhookAddress) Label() string { return "Webhook " + term.Sanitize(a.Path) }

  // Applied reports whether the response carried an applicationCapabilities object (null counts as absent).
  func Applied(applied json.RawMessage) bool {
      t := bytes.TrimSpace(applied)
      return len(t) > 0 && !bytes.Equal(t, []byte("null"))
  }

  // WebhookAddresses lists every string field named "address" of the applied block, sorted by dotted path.
  func WebhookAddresses(applied json.RawMessage) []WebhookAddress {
      var root any
      if json.Unmarshal(applied, &root) != nil { return nil }
      var out []WebhookAddress
      collectAddresses(root, "", &out)
      slices.SortFunc(out, func(a, b WebhookAddress) int { return strings.Compare(a.Path, b.Path) })
      return out
  }

  func collectAddresses(v any, path string, out *[]WebhookAddress) {
      switch v := v.(type) {
      case map[string]any:
          for key, child := range v {
              if s, ok := child.(string); ok && key == "address" {
                  *out = append(*out, WebhookAddress{Path: cmp.Or(path, key), Address: s})
                  continue
              }
              collectAddresses(child, joinPath(path, key), out)
          }
      case []any:
          for i, child := range v { collectAddresses(child, joinPath(path, strconv.Itoa(i)), out) }
      }
  }
  ```
  (`joinPath` is `key` when `path == ""`, else `path + "." + key`.) It never sanitises: callers sanitise the address,
  `Label` sanitises the path.
  - Verify: `cd <WT> && go test ./internal/platform/` passes.
- [x] 2.4 `internal/debugsession` compiles again with the new signature (no behaviour yet):
  - `internal/debugsession/debugsession.go:19` interface: add `appCaps map[string]any` as the last parameter.
  - Call sites `run.go:133` and `rich.go:21`: pass `nil` (group 4 passes the block).
  - Fake `internal/debugsession/localapi_test.go:74`: add the parameter; add `AppCaps map[string]any` to `deployCall` (:31-35)
    and fill it. Callers `localapi_self_test.go:21,49,61`: add `, nil`.
  - Verify: `cd <WT> && go build ./... && go vet ./... && go test ./internal/platform/ ./internal/debugsession/`.
- [x] 2.5 Commit: `feat: let the platform client send and read an application-capabilities block`.
  Before it: `cd <WT> && make test`.

## 3. Deploy

- [x] 3.1 Failing tests in new `internal/cli/deploy/appcaps_test.go` (package `deploy`; helpers `newEnv`, `newEnvAt`, `execute`,
  `manifestYAML`, `writeProject`, `requestsOf`, `requestNamed`, `jsonBody`, `defaultSettings`, `good` from `env_test.go` and
  `fake_test.go`; the fake platform needs no change: it ignores the new request field and answers without the new
  response field).
  - Constants: `blockYAML` is the top-level block
    `application-capabilities:\n  voice:\n    signed_callbacks: true\n    webhooks:\n      answer_url: {address: "{{instance_url}}/answer", http_method: POST}\n`;
    `blockJSON` is `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":{"address":"{{instance_url}}/answer","http_method":"POST"}}}}`.
    Pass the block through the `extra` argument of `manifestYAML`; a line without indentation after the instance lines
    starts a top-level key (for example `"  capabilities: [voice]\n" + blockYAML`).
  - `TestBlockIsRefusedBeforeAnyRequest` (table; each row: `srv := fakeplatform.Start(t, fakeplatform.Defaults())`,
    `env, stdout, _ := newEnv(...)`, `err := execute(t, env, append([]string{dir}, args...)...)`; asserts
    `assert.Empty(t, requestsOf(srv))` (not even the region lookup), `assert.Empty(t, stdout.String())`):

    | name | manifest extra | args | want |
    |---|---|---|---|
    | both in the manifest | `"  capabilities: [voice]\n" + blockYAML` | none | `require.ErrorIs(err, manifest.ErrBlockAndList)` |
    | flag and block | `blockYAML` | `-c voice` | `ErrorIs ErrBlockAndFlag` |
    | long flag and block | `blockYAML` | `--capabilities voice,rtc` | `ErrorIs ErrBlockAndFlag` |
    | unknown token | `blockYAML` with `{{instance_host}}/answer` | none | `EqualError` `application-capabilities uses unknown tokens: {{instance_host}}; the only valid token is {{instance_url}}` |
    | block with an invalid capability entry | `"  capabilities: [voicemail]\n" + blockYAML` | none | `ErrorIs ErrBlockAndList` (exclusivity wins over the later capability parse) |
  - `TestBlockIsSentAsWritten`: manifest `blockYAML`, no capabilities; `require.NoError`; `create := requestNamed(t, srv, "POST /packages")`;
    `require.JSONEq(t, `{"sourceCodeKey":"source-key-0001","entrypoint":["node","index.js"],"capabilities":{},"buildScriptPath":"","runtime":"nodejs18","applicationCapabilities":`+blockJSON+`}`, string(create.Body))`;
    the `POST /deployments` body has no `applicationCapabilities` key (`assert.NotContains(t, jsonBody(t, deployReq), "applicationCapabilities")`).
    Rows: v0.3, v0.4 and v0.5 via `newEnvAt` (the field is not version gated); a row with the literal address
    `https://example.com/answer` (block sent unchanged).
  - `TestInstanceCapabilitiesWithoutABlockSendNoBlock`: manifest `capabilities: [voice]`, no block; `jsonBody(create)` has
    `capabilities == {"voice": "v0"}` and `assert.NotContains(t, body, "applicationCapabilities")`.
  - `TestBlockAgainstAPlatformWithoutSupportWarns`: manifest `blockYAML`; `require.NoError`; `stderr.String()` equals
    `"! " + platform.NotAppliedNotice + "\n"`; `stdout.String()` ends with the usual
    `| Instance host address: ...` / `\-------` block and `assert.NotContains(t, stdout.String(), "Webhook ")`.
  - Verify: `cd <WT> && go test ./internal/cli/deploy/` fails.
- [x] 3.2 Wire the check into the command.
  - `internal/cli/deploy/plan.go`: add
    ```go
    // flagCapabilities is the --capabilities value split on commas as typed, or nil when the flag is empty.
    func flagCapabilities(v string) []string {
        if v == "" { return nil }
        return strings.Split(v, ",")
    }
    ```
    and in `buildPlan` (:42-44) use it: `if caps := flagCapabilities(f.capabilities); caps != nil { merged.Instance.Capabilities = caps }`
    (same behaviour; the existing `TestCapabilitiesFlagReplacesTheManifestList` and
    `TestCapabilitiesStayNilWhenNothingNamesAny` prove it).
  - `internal/cli/deploy/deploy.go:67-70`: after the `manifest.Read` error branch and before `effectiveRegion` (:72):
    ```go
    if err := manifest.CheckApplicationCapabilities(read, flagCapabilities(f.capabilities)); err != nil {
        return err
    }
    ```
    The error is returned as is (no prefix), so `clierr.Render` prints it once and the exit code is 1.
  - Verify: the refusal rows of 3.1 pass.
- [x] 3.3 Failing tests in new `internal/deploy/appcaps_test.go` (package `deploy`; `newHarness`, `fakeAPI`, `finalBlock`,
  `wantBlock`, `hostLine` from `deploy_test.go`, `fake_api_test.go`, `summary_test.go`; `newRichRun`, `lines`,
  `squashed`, `blockFrom` from `rich_test.go`).
  - Add helper `webhookLine(path, address string) string { return "| Webhook " + path + ": " + term.YellowBold(address) + "\n" }`.
  - `TestPackageRequestCarriesTheBlock`: `h.plan.Manifest = &manifest.Manifest{Instance: ..., ApplicationCapabilities: block}`;
    after `h.run()`: `assert.Equal(t, block, h.api.packageReq.ApplicationCapabilities)` and
    `assert.Equal(t, manifest.Capabilities{}, h.api.packageReq.Capabilities)`. Without a block `assert.Nil(...)`; with `{}`
    `assert.Empty(...)`.
  - `TestSummaryListsTheAppliedWebhookAddresses` (classic; set `h.api.deployed.ApplicationCapabilities = json.RawMessage(...)`;
    compare `finalBlock(h.stdout.String())`):

    | name | applicationCapabilities | host URLs | want block |
    |---|---|---|---|
    | spec example | `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}` | default two | header lines, both `hostLine`s, then `webhookLine("voice.webhooks.answer_url", "https://h.example/answer")`, `webhookLine("voice.webhooks.event_url", "https://h.example/event")`, then `\-------` |
    | no host URLs | same | none | header lines, the two webhook lines, `\-------` |
    | only addresses | `{"voice":{"webhooks":{"answer_url":{"address":"https://h/a","http_method":"POST"}},"signed_callbacks":true},"rtc":null}` | default two | one webhook line |
    | applied object without addresses | `{"rtc":{}}` | default two | `wantBlock()` unchanged |
    | absent | not set | default two | `wantBlock()` unchanged |
    | platform text is sanitised | `{"voice":{"x\u001b[31my":{"address":"https://h.\u001b[2Jexample/a"}}}` | default two | `webhookLine("voice.xy", "https://h.example/a")` |
  - `TestRichSummaryListsTheAppliedWebhookAddresses` for `term.Rich` and `term.Static`: `r := newRichRun(t, mode)`;
    `r.api.deployed.HostURLs = []string{"https://one.example.test"}`; response as the spec example; then
    `blockFrom(t, squashed(lines(r.stdout.Screen())), "✓ Instance has been deployed!")` equals
    `["✓ Instance has been deployed!", "  Instance id: i-1", "  Instance service name: my-project-dev", "  Instance host address: https://one.example.test", "  Webhook voice.webhooks.answer_url: https://h.example/answer", "  Webhook voice.webhooks.event_url: https://h.example/event"]`.
    Without the field the existing `TestRichDeployResolvesEachStage` expectation (no webhook row) is the proof.
  - `TestDeployWarnsWhenTheBlockWasNotApplied` (table; plan block set unless noted; `h.stderr.String()` and `h.stdout.String()`):

    | name | block sent | response `applicationCapabilities` | stderr | stdout |
    |---|---|---|---|---|
    | platform without support | yes | absent | `"! " + platform.NotAppliedNotice + "\n"` | `wantBlock()` tail |
    | null | yes | `null` | the notice | `wantBlock()` tail |
    | applied, nothing to list | yes | `{}` | empty | `wantBlock()` tail |
    | applied | yes | the spec example | empty | block with webhook lines |
    | no block, no field | no | absent | empty | `wantBlock()` tail |
    | no block, field present | no | the spec example | empty | block with webhook lines |

    Each `require.NoError(t, h.run())`: the deploy exits 0. One extra rich row (`newRichRun`, `term.Rich`):
    `assert.Contains(t, r.stderr.Raw(), platform.NotAppliedNotice)` and the stdout screen is the usual one.
  - Verify: `cd <WT> && go test ./internal/deploy/` fails.
- [x] 3.4 Send, show and warn.
  - `internal/deploy/stages.go:205-212` `packageRequest`: add `ApplicationCapabilities: r.p.Manifest.ApplicationCapabilities,`
    to the `platform.PackageRequest` literal (`Run` already guarantees `Manifest` is non-nil, `run.go:18-20`). No version check.
  - `internal/deploy/summary.go:26-29` classic: after the host loop and before `\-------`:
    ```go
    for _, a := range platform.WebhookAddresses(deployed.ApplicationCapabilities) {
        b.WriteString(line(yellow("|"), yellow(a.Label()+":"), term.YellowBold(term.Sanitize(a.Address))))
    }
    ```
  - `internal/deploy/rich.go:187-189` `summaryRich`: after the host loop,
    `rows = append(rows, term.Row{Label: a.Label(), Value: term.Sanitize(a.Address)})` for each address.
  - `internal/deploy/summary.go`: new
    ```go
    // warnNotApplied says on stderr that a block that was sent has no counterpart in the response.
    func (r *runner) warnNotApplied(deployed platform.DeployResult) {
        if len(r.p.Manifest.ApplicationCapabilities) == 0 || platform.Applied(deployed.ApplicationCapabilities) { return }
        fmt.Fprintf(r.t.ErrOut, "%s %s\n", r.c.WarningIcon(), platform.NotAppliedNotice)
    }
    ```
    and in `internal/deploy/run.go:67-71` call `r.warnNotApplied(deployed)` between `r.deploy(...)` and `r.summary(deployed)`,
    so the warning comes before the summary block. Update the doc comment of `Run` (:12-16) to mention the webhook lines.
  - Verify: `cd <WT> && go test ./internal/deploy/ ./internal/cli/deploy/ ./internal/platform/`.
- [x] 3.5 Commit: `feat: send the application-capabilities block on vcr deploy and show the addresses applied`.
  Before it: `cd <WT> && make test`.

## 4. Debug

- [x] 4.1 Failing tests in new `internal/cli/debug/appcaps_test.go` (package `debug`; `newFixture`, `fixture.project`,
  `fixture.run`, `fullManifest`, `instanceOnlyManifest`, `instanceAppID`, `debugAppID`, `choose`, `fallbackAppIDWarning`
  from `helpers_test.go`, `prompt_test.go`, `resolve_test.go`). The session is the `recorder`, so "no debug-session
  request" is `f.rec.count() == 0`, and `f.srv.Requests()` shows the region lookup.
  - Constant `blockYAML` (top-level): `application-capabilities: {voice: {webhooks: {answer_url: {address: "{{instance_url}}/answer", http_method: POST}}, signed_callbacks: true}}\n`.
    A manifest with a block is `fullManifest + blockYAML` or `instanceOnlyManifest + blockYAML` (a top-level key may follow the
    `debug:` flow mapping).
  - `TestBlockChecksRunBeforeTheRegionIsLooked` (`newFixture(t, false)`):

    | name | manifest | want |
    |---|---|---|
    | block and instance.capabilities | `fullManifest` with `capabilities: [voice]` added to the instance line, plus `blockYAML` | `ErrorIs manifest.ErrBlockAndList` |
    | unknown token | `fullManifest + blockYAML` with `{{instance_host}}` | `ErrorContains` `{{instance_host}}` and `{{instance_url}}` |

    Each also `assert.Zero(t, f.rec.count())`, `assert.Empty(t, f.srv.Requests())`, `assert.Empty(t, f.stdout.String())`.
    (`vcr debug` has no `--capabilities` flag, so only the manifest pair applies.)
  - `TestBlockGuardsTheInstanceApplication` (table). Pin the text once in `TestGuardErrorText`:
    `assert.EqualError(t, errInstanceApplication, "application-capabilities would point the webhooks of the instance application at this debug session: set debug.application-id or --app-id to a different application")`.

    | name | fixture | manifest | args | want |
    |---|---|---|---|---|
    | falls back to the instance application, --yes | `newFixture(t, false)` | `instanceOnlyManifest + blockYAML` | `--yes` | `ErrorIs errInstanceApplication`; `rec.count() == 0`; `f.stdout.String() == fallbackAppIDWarning` (the notice still prints first) |
    | falls back, no terminal, no flag | `newFixture(t, false)` | `instanceOnlyManifest + blockYAML` | none | same error |
    | the flag names the instance application | `newFixture(t, false)` | `fullManifest + blockYAML` | `--app-id <instanceAppID>` | same error |
    | the prompt is answered yes | `newFixture(t, true)`, `f.prompt.yesNo = []bool{true}` | `instanceOnlyManifest + blockYAML` | none | same error; `f.prompt.questions == []string{wantInstanceQuestion}` |
    | the picker chooses another application | `newFixture(t, true)`, `yesNo = []bool{false}`, `f.prompt.pick = choose("second-app")` (as `prompt_test.go:128`) | `instanceOnlyManifest + blockYAML` | none | no error; `rec.only(t).cfg.AppID != instanceAppID` |
    | separate debug application | `newFixture(t, false)` | `fullManifest + blockYAML` | none | no error; `rec.only(t).cfg.AppID == debugAppID`; `cfg.Manifest.ApplicationCapabilities` equals the block as nested maps |
    | no block, fallback unchanged | `newFixture(t, false)` | `instanceOnlyManifest` | `--yes` | no error; `cfg.AppID == instanceAppID`; stdout is `fallbackAppIDWarning` |
  - Verify: `cd <WT> && go test ./internal/cli/debug/` fails.
- [x] 4.2 Wire the checks into the command.
  - `internal/cli/debug/resolve.go:44-47` `execute`: after `loadProject` and before `chooseRegion` (:48):
    ```go
    if err := manifest.CheckApplicationCapabilities(m, nil); err != nil {
        return err
    }
    ```
  - `resolve.go:26-29` add to the `var` block:
    `errInstanceApplication = errors.New("application-capabilities would point the webhooks of the instance application at this debug session: set debug.application-id or --app-id to a different application")`.
  - `resolve.go:92-94` `config`, directly after `confirmApplication` returns (the prompt may have changed the id), before
    building the `Config`:
    ```go
    if len(m.ApplicationCapabilities) > 0 && appID == m.Instance.ApplicationID {
        return debugsession.Config{}, errInstanceApplication
    }
    ```
    An error, not a prompt: `--yes` and non-terminal runs skip prompts. `Config` needs no new field: the session reads
    `Manifest.ApplicationCapabilities`.
  - Verify: the rows of 4.1 pass.
- [x] 4.3 Failing tests in new `internal/debugsession/appcaps_test.go` (package `debugsession`; `sharedSession`, `testConfig`,
  `startRun`, `waitForOutput`, `helperReady`, `newLocalSetup`, `testIO`, `lockedBuffer`, `startRichRun` from
  `run_harness_test.go`, `rich_helpers_test.go`, `helper_test.go`).
  - `TestDeployRequestCarriesTheBlock`: like `TestDeployRequestNamesTheRegionAndTheApplication` (`run_test.go:75`) with
    `cfg.AppID = "app-1"` (not an instance application) and `cfg.Manifest.ApplicationCapabilities = <block>`; the decoded
    `POST /debug/services` body equals `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","capabilities":{},"applicationCapabilities":<block as map[string]any>}`.
    The existing test, unchanged, is the "no block" proof (exact map, no `applicationCapabilities` key).
  - `TestSessionWarnsWhenTheBlockWasNotApplied`: the same run; the shared fake answers without the field, so
    `run.errOut.String() == "! "+platform.NotAppliedNotice+"\n"` after `waitForOutput(t, run.out, helperReady)` and `run.stop()`;
    stdout still equals the usual intro (no `Webhook` line). The existing `TestSessionLifecycleOutput`
    (`assert.Empty(t, run.errOut.String())`) is the no-block proof.
  - `TestIntroListsTheAppliedWebhookAddresses` (classic, bare session like `intro_test.go:36-45`, with
    `service: platform.DebugService{ServiceName: "svc", ApplicationCapabilities: json.RawMessage(...)}`):

    | name | applicationCapabilities | want `out.String()` |
    |---|---|---|
    | spec example | `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}` | `introBlock(style, "svc", host, answer, event)`: ends `...Application Host: <host>\n| Webhook voice.webhooks.answer_url: <YellowBold url>\n| Webhook voice.webhooks.event_url: <YellowBold url>\n\-------\n\n` |
    | absent | not set | `introBlock(style, "svc", host)` unchanged |
    | sanitised | `{"voice":{"x\u001b[31my":{"address":"https://h.\u001b[2Jexample/a"}}}` | contains `| Webhook voice.xy: ` and no `\x1b[2J` |
    | not applied | set `s.appCaps = map[string]any{"voice": map[string]any{}}`, field absent | stdout is the usual block; `errOut.String() == "! "+platform.NotAppliedNotice+"\n"` |
    | block not sent | `s.appCaps` nil, field absent | `errOut` empty |
  - `TestRichIntroListsTheAppliedWebhookAddresses`: `tio, out, errOut := term.TestRich(term.Rich, 100, clocktest.New(testStart))`,
    bare session as above, `s.printIntro()`; `requireLines(t, screenLines(out.Screen()), ...)` with the usual lines plus
    `^  Webhook voice\.webhooks\.answer_url:?\s+https://h\.example/answer$` and the `event_url` twin after the `Host` row;
    a second row without the field has no such line; `errOut.Raw()` is empty.
  - Existing `introBlock` calls in `intro_test.go` and `run_test.go:50` must keep compiling unchanged (variadic last argument).
  - Verify: `cd <WT> && go test ./internal/debugsession/` fails.
- [x] 4.4 Send, show and warn in the session.
  - `internal/debugsession/run.go` `session` (:34-57): add `appCaps map[string]any // the manifest's block, sent on the debug-service request`.
    `preflight` (:97-115): after the capabilities parse, `s.appCaps = m.ApplicationCapabilities` (no check here; see Plan notes).
  - `run.go:133` and `rich.go:21`: pass `s.appCaps` instead of `nil`.
  - `internal/debugsession/intro.go:7-15`:
    ```go
    func introBlock(style term.Style, serviceName, hostText string, webhooks ...platform.WebhookAddress) string {
        // ... existing lines up to "| Application Host:" ...
        for _, a := range webhooks {
            b += yellow("| "+a.Label()+":") + " " + term.YellowBold(term.Sanitize(a.Address)) + "\n"
        }
        // ... then yellow(`\-------`) + "\n\n"
    ```
    (build with a `strings.Builder` or concatenate; keep the existing lines byte for byte.) `printIntro` (:18-24):
    call `s.warnNotApplied()` first, then pass `platform.WebhookAddresses(s.service.ApplicationCapabilities)...` to `introBlock`.
    `printIntroRich` (`rich.go:53-65`): after the `Host` row, append
    `term.Row{Label: a.Label(), Value: style.Bold(term.Sanitize(a.Address))}` for each address. New in `intro.go`:
    ```go
    func (s *session) warnNotApplied() {
        if len(s.appCaps) == 0 || platform.Applied(s.service.ApplicationCapabilities) { return }
        s.con.stderr(fmt.Sprintf("%s %s\n", s.con.style.WarningIcon(), platform.NotAppliedNotice))
    }
    ```
    It reads `s.appCaps`, not `s.cfg.Manifest`, so existing bare-session intro tests (which have no `cfg`) cannot nil-dereference.
  - Verify: `cd <WT> && go test ./internal/debugsession/ ./internal/cli/debug/`.
- [x] 4.5 Commit: `feat: send the application-capabilities block from vcr debug and guard the instance application`.
  Before it: `cd <WT> && make test`.

## 5. Check

- [ ] 5.1 Run `cd <WT> && make lint test contract contract-rich docs`. Verify: all pass.
  - `make test` is `go test -race ./...`; CI repeats it on Linux, macOS and Windows. Nothing in this change builds paths or
    depends on newlines beyond Go's `\n`: the new tests compare parsed values, never YAML bytes, and the terminal tests use
    the same buffers as the existing ones. Do not add `filepath` or `os` assumptions to them. `make lint` also lints
    `GOOS=windows`; the new files have no OS-specific code.
  - `make contract` and `make contract-rich` build the CLI with the harness's `v0.3` stamp and replay every recorded scenario
    against the fake platform. They stay green with no change because: no scenario or corpus manifest has an
    `application-capabilities` key (`git grep -n application-capabilities -- test/` prints nothing); every new request field is
    `omitempty` and absent without a block, so request bodies are byte-identical; the fake platform answers without the new
    response field, so no webhook line and no warning is printed; the classic and rich summary and intro layouts change only
    when a response carries the object; the new label is `Webhook <path>`, not `Instance host address`, so the scraped
    patterns in `test/contract/scraped.yaml` see no new line; and nothing is gated on the API version, so the `v0.3` stamp
    hides nothing. Rules while doing this:
    - add no line to `test/contract/allowed-diffs.txt`; a contract failure means a regression in this change, so fix the code;
    - never run `make contract-record` or `make contract-rich-record`, and never edit `test/contract/testdata/`;
    - add no scenario (design: transcripts come from the baseline, which lacks the feature).
  - `make docs` regenerates the command reference from the command tree; there is no flag and no help text change, so it must
    produce no diff.
  - `cd <WT> && git status --short docs/ test/` prints nothing; `git diff --stat origin/main` lists only `internal/` files and
    the change folder.
- [ ] 5.2 Check the diff for anything a public repository must not carry: ticket keys, internal hostnames or URLs, internal
  service or repository names, plan ids, tool names. Verify:
  read `cd <WT> && git diff origin/main` once through, and `git diff origin/main | grep -nE '[A-Z]{2,}-[0-9]+'` prints nothing
  (ticket-key shape).
- [ ] 5.3 Run `cd <WT> && openspec validate application-capabilities --strict`. Verify: it passes.
- [ ] 5.4 Archive the change as the last commit: `docs(openspec): archive application-capabilities`.
  Verify: `cd <WT> && make test` still passes.

## Spec traceability

Test paths are under `<WT>/internal/`. "Case" is the table row or subtest name.

| Requirement | Scenario | Test function (file) and case |
|---|---|---|
| The manifest carries an application-capabilities block | Block read as written | `TestParseReadsTheBlockAsWritten`, case "the spec block" (`manifest/appcaps_test.go`); wire: `TestCreatePackageSendsTheBlockOnEveryVersion` (`platform/appcaps_test.go`) |
| The manifest carries an application-capabilities block | Template manifest keeps the block | `TestTemplateManifestKeepsTheApplicationCapabilitiesBlock` (`cli/initcmd/template_test.go`); plus `TestMarshalKeepsTheBlock`, `TestMergeKeepsAndDeepCopiesTheBlock` (`manifest/appcaps_test.go`) |
| The block and the capabilities list are exclusive | Both in the manifest | `TestCheckApplicationCapabilities`, case "block and instance.capabilities" (`manifest/appcaps_test.go`); `TestBlockIsRefusedBeforeAnyRequest`, case "both in the manifest" (`cli/deploy/appcaps_test.go`); `TestBlockChecksRunBeforeTheRegionIsLooked`, case "block and instance.capabilities" (`cli/debug/appcaps_test.go`) |
| The block and the capabilities list are exclusive | Flag and block | `TestCheckApplicationCapabilities`, case "block and flag"; `TestBlockIsRefusedBeforeAnyRequest`, cases "flag and block" and "long flag and block" |
| Only the instance_url token is accepted | Unknown token | `TestCheckApplicationCapabilities`, case "unknown token" (and the nested, sorted, valid-and-unknown cases); `TestBlockIsRefusedBeforeAnyRequest`, case "unknown token"; `TestBlockChecksRunBeforeTheRegionIsLooked`, case "unknown token" |
| Only the instance_url token is accepted | Literal address | `TestCheckApplicationCapabilities`, case "literal address"; `TestBlockIsSentAsWritten`, case "literal address" (`cli/deploy/appcaps_test.go`) |
| Deploy sends the block | Block sent | `TestBlockIsSentAsWritten`, cases v0.3, v0.4, v0.5 (`cli/deploy/appcaps_test.go`: `applicationCapabilities` equals the block, `capabilities` is `{}`); `TestPackageRequestCarriesTheBlock` (`deploy/appcaps_test.go`); `TestCreatePackageSendsTheBlockOnEveryVersion` (`platform/appcaps_test.go`) |
| Deploy sends the block | No block | `TestInstanceCapabilitiesWithoutABlockSendNoBlock` (`cli/deploy/appcaps_test.go`); `TestCreatePackageWithoutABlockKeepsItsBytes` (`platform/appcaps_test.go`); existing `TestDeployRequestBodies` (`cli/deploy/fake_test.go`) |
| Deploy shows the applied webhook addresses | Addresses printed | `TestSummaryListsTheAppliedWebhookAddresses`, case "spec example" (classic); `TestRichSummaryListsTheAppliedWebhookAddresses` (rich and static) (`deploy/appcaps_test.go`); `TestWebhookAddresses`, case "the spec example" (`platform/appcaps_test.go`) |
| Deploy shows the applied webhook addresses | Platform without support | `TestDeployWarnsWhenTheBlockWasNotApplied`, cases "platform without support" and "null" (`deploy/appcaps_test.go`); `TestBlockAgainstAPlatformWithoutSupportWarns` (`cli/deploy/appcaps_test.go`, exit 0, usual summary, stderr notice) |
| Debug sends the block and guards the production application | Debug falls back to the instance application | `TestBlockGuardsTheInstanceApplication`, cases "falls back to the instance application, --yes", "falls back, no terminal, no flag", "the flag names the instance application", "the prompt is answered yes" (`cli/debug/appcaps_test.go`; `rec.count() == 0` is "no debug-session request"); `TestGuardErrorText` |
| Debug sends the block and guards the production application | Separate debug application | `TestBlockGuardsTheInstanceApplication`, cases "separate debug application" and "the picker chooses another application"; `TestDeployRequestCarriesTheBlock` (`debugsession/appcaps_test.go`: the request carries `applicationCapabilities`); `TestDeployDebugServiceSendsTheBlock` (`platform/appcaps_test.go`) |
| Debug sends the block and guards the production application | No block | `TestBlockGuardsTheInstanceApplication`, case "no block, fallback unchanged"; existing `TestApplicationIDPrecedenceAndWarning`, case "the instance id says so" (`cli/debug/resolve_test.go`); existing `TestDeployRequestNamesTheRegionAndTheApplication` (`debugsession/run_test.go`) |

The same requirement's remaining sentence (debug prints the applied addresses or the not-applied warning as deploy does) is
covered by `TestIntroListsTheAppliedWebhookAddresses`, `TestRichIntroListsTheAppliedWebhookAddresses` and
`TestSessionWarnsWhenTheBlockWasNotApplied` (`debugsession/appcaps_test.go`).

## Workflow follow-up

- Release only after the platform support is live in every region. A release before that is safe (the platform ignores
  the field, nothing is configured, and the CLI warns), but customers would see the warning for a feature that does nothing.
- PR title: `feat: apply raw Vonage application capabilities from vcr.yml`; the body is one line pointing at the change
  folder plus the output of `make lint test contract contract-rich docs`.
- Document the block in the public Cloud Runtime docs; `vcr deploy --help` and the generated reference stay as they are.
