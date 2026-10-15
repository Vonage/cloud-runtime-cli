//go:build !windows

package contract

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

type reqOpt func(*fakeplatform.Request)

func mkReq(kind, method, rel string, opts ...reqOpt) fakeplatform.Request {
	r := fakeplatform.Request{
		Listener: "primary", Method: method, Path: rel, Header: http.Header{}, Kind: kind,
		Region: "euw1", Rel: rel, Route: method + " " + rel,
	}
	for _, o := range opts {
		o(&r)
	}
	return r
}

func withBasic(user string) reqOpt {
	return func(r *fakeplatform.Request) {
		r.User = user
		r.Header.Set("Authorization", "Basic dGVzdA==")
		r.Header.Set("X-Neru-ApiAccountId", user)
	}
}

func withHeader(k, v string) reqOpt { return func(r *fakeplatform.Request) { r.Header.Set(k, v) } }

func withBody(ctype string, body []byte) reqOpt {
	return func(r *fakeplatform.Request) {
		r.Body = body
		r.Header.Set("Content-Type", ctype)
	}
}

func withQuery(q string) reqOpt { return func(r *fakeplatform.Request) { r.RawQuery = q } }

func render(reqs ...fakeplatform.Request) string {
	return RenderRequests(Observed{Requests: reqs}, Normaliser{})
}

func TestRenderRESTRequest(t *testing.T) {
	cases := []struct {
		name string
		req  fakeplatform.Request
		want string
	}{
		{"get with trace", mkReq("rest", "GET", "/secrets", withBasic("abc123"), withHeader("X-Neru-TraceId", "t-1")),
			"euw1 GET /secrets\n  auth: basic abc123\n  acct: abc123\n  trace: yes\n"},
		{"json body is canonical", mkReq("rest", "POST", "/secrets", withBasic("abc123"), withBody("application/json; charset=utf-8", []byte(`{"b": 1, "a": 2}`))),
			"euw1 POST /secrets\n  auth: basic abc123\n  acct: abc123\n  trace: no\n  type: application/json\n  body: {\"a\":2,\"b\":1}\n"},
		{"no credentials", mkReq("rest", "GET", "/applications"),
			"euw1 GET /applications\n  auth: none\n  acct: none\n  trace: no\n"},
		{"other scheme", mkReq("rest", "GET", "/applications", withHeader("Authorization", "Bearer x")),
			"euw1 GET /applications\n  auth: other\n  acct: none\n  trace: no\n"},
		{"query pairs are sorted", mkReq("rest", "GET", "/instances", withBasic("abc123"), withQuery("b=2&a=1&a=0")),
			"euw1 GET /instances?a=0&a=1&b=2\n  auth: basic abc123\n  acct: abc123\n  trace: no\n"},
		{"text body", mkReq("rest", "PUT", "/x", withBody("text/plain", []byte("plain text"))),
			"euw1 PUT /x\n  auth: none\n  acct: none\n  trace: no\n  type: text/plain\n  body: plain text\n"},
		{"multi line text body stays on one line", mkReq("rest", "PUT", "/x", withBody("text/plain", []byte("a\n\nb"))),
			"euw1 PUT /x\n  auth: none\n  acct: none\n  trace: no\n  type: text/plain\n  body: a\\n\\nb\n"},
		{"binary body", mkReq("rest", "PUT", "/x", withBody("application/octet-stream", []byte("\xff\x00\x01"))),
			"euw1 PUT /x\n  auth: none\n  acct: none\n  trace: no\n  type: application/octet-stream\n  body: sha256=" + sumOf("\xff\x00\x01") + " size=3\n"},
		{"large body", mkReq("rest", "PUT", "/x", withBody("text/plain", bytes.Repeat([]byte("x"), 5000))),
			"euw1 PUT /x\n  auth: none\n  acct: none\n  trace: no\n  type: text/plain\n  body: sha256=" + sumOf(strings.Repeat("x", 5000)) + " size=5000\n"},
		{"no region is a dash", mkReq("rest", "GET", "/x", func(r *fakeplatform.Request) { r.Region = "" }),
			"- GET /x\n  auth: none\n  acct: none\n  trace: no\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, render(c.req)) })
	}
	two := render(mkReq("rest", "GET", "/a"), mkReq("rest", "GET", "/b"))
	assert.Equal(t, "euw1 GET /a\n  auth: none\n  acct: none\n  trace: no\n\neuw1 GET /b\n  auth: none\n  acct: none\n  trace: no\n", two)
	assert.Empty(t, render())
}

func tarGz(t *testing.T, level int, name string, mtime time.Time) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, level)
	require.NoError(t, err)
	zw.Name, zw.ModTime = name, mtime
	tw := tar.NewWriter(zw)
	entries := []struct {
		hdr  tar.Header
		body string
	}{
		{tar.Header{Name: "src/hard", Typeflag: tar.TypeLink, Linkname: "src/b.txt", Mode: 0o644}, ""},
		{tar.Header{Name: "src/b.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3, ModTime: mtime}, "bbb"},
		{tar.Header{Name: "src/a.link", Typeflag: tar.TypeSymlink, Linkname: "b.txt", Mode: 0o777}, ""},
		{tar.Header{Name: "src/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: mtime}, ""},
	}
	for _, e := range entries {
		h := e.hdr
		require.NoError(t, tw.WriteHeader(&h))
		_, err := tw.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func multipartBody(t *testing.T, archive []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.SetBoundary("testboundary123"))
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="source.tgz"`)
	h.Set("Content-Type", "application/gzip")
	part, err := mw.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(archive)
	require.NoError(t, err)
	require.NoError(t, mw.WriteField("meta", "some text"))
	require.NoError(t, mw.Close())
	return mw.FormDataContentType(), buf.Bytes()
}

func TestRenderMultipartListsSortedTarEntries(t *testing.T) {
	older := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 6, 6, 6, 6, 0, time.UTC)
	ctA, bodyA := multipartBody(t, tarGz(t, gzip.BestSpeed, "", older))
	ctB, bodyB := multipartBody(t, tarGz(t, gzip.BestCompression, "named", newer))
	require.NotEqual(t, bodyA, bodyB, "the two encodings differ in bytes")

	post := func(ct string, body []byte) string {
		return render(mkReq("rest", "POST", "/packages/source", withBasic("abc123"), withBody(ct, body)))
	}
	want := "euw1 POST /packages/source\n  auth: basic abc123\n  acct: abc123\n  trace: no\n" +
		"  type: multipart/form-data\n" +
		"  part: name=file filename=source.tgz\n" +
		"  tar: 4 entries\n" +
		"    dir 0755 0 src/\n" +
		"    symlink ---- 0 src/a.link -> b.txt\n" +
		"    file 0644 3 src/b.txt\n" +
		"    hardlink 0644 0 src/hard -> src/b.txt\n" +
		"  part: name=meta filename=\n" +
		"  raw: sha256=" + sumOf("some text") + " size=9\n"
	assert.Equal(t, want, post(ctA, bodyA))
	assert.Equal(t, want, post(ctB, bodyB), "different gzip bytes of the same entries render the same")
}

func TestRenderGraphQLAndMarket(t *testing.T) {
	gql := mkReq("graphql", "POST", "", withBasic("abc123"), withBody("application/json", []byte(`{"variables":{},"query":"q"}`)), func(r *fakeplatform.Request) {
		r.Route, r.Rel = "POST graphql:Regions", ""
	})
	market := mkReq("market", "GET", "/products/abc/template.tgz", func(r *fakeplatform.Request) { r.Region = "" })
	ws := mkReq("websocket", "GET", "/packages/p1/build/watch", withBasic("abc123"))
	other := mkReq("debug", "GET", "/hello", func(r *fakeplatform.Request) { r.Listener, r.Region = "other", "" })
	otherWS := mkReq("websocket", "GET", "/0123", func(r *fakeplatform.Request) { r.Listener, r.Region = "other", "" })

	got := render(gql, market, ws, other, otherWS)
	assert.Contains(t, got, "euw1 POST graphql:Regions\n  auth: basic abc123\n  acct: abc123\n  trace: no\n  type: application/json\n  body: {\"query\":\"q\",\"variables\":{}}\n")
	assert.Contains(t, got, "- GET market:/products/abc/template.tgz\n  auth: none\n")
	assert.Contains(t, got, "euw1 WS /packages/p1/build/watch\n  auth: basic abc123\n")
	assert.Contains(t, got, "other GET /hello\n  auth: none\n")
	assert.Contains(t, got, "other WS /0123\n")
}

func TestReleaseLaneIsListedLast(t *testing.T) {
	gh := func(rel string) fakeplatform.Request {
		return mkReq("github", "GET", rel, func(r *fakeplatform.Request) { r.Region = "" })
	}
	got := render(gh("/releases/latest"), mkReq("rest", "GET", "/secrets"), gh("/releases/tags/v1"), mkReq("rest", "GET", "/applications"))
	var heads []string
	for _, b := range strings.Split(strings.TrimSuffix(got, "\n"), "\n\n") {
		heads = append(heads, strings.SplitN(b, "\n", 2)[0])
	}
	assert.Equal(t, []string{"euw1 GET /secrets", "euw1 GET /applications", "gh GET /releases/latest", "gh GET /releases/tags/v1"}, heads)
}

func TestRenderPeerLocalAndPublicSubBlocks(t *testing.T) {
	exec := &fakeplatform.PeerMessage{Operation: "execute", Request: &fakeplatform.PeerRequest{ID: "public-1", Operation: "execute", Method: "GET", Route: "/hello"}}
	resp := &fakeplatform.PeerMessage{Operation: "execute-response", Response: &fakeplatform.PeerResponse{ID: "public-1", Operation: "execute-response", Status: 200, Payload: []byte("hello from app")}}
	remote := &fakeplatform.PeerMessage{Operation: "execute-remote", Remote: &fakeplatform.PeerRemoteRequest{ID: "r1", Operation: "execute-remote",
		Request: fakeplatform.PeerRemoteCommand{FAASFunction: "fn", URL: "http://x/y", Method: "POST", Payload: []byte("p")}}}
	answer := &fakeplatform.PeerMessage{Operation: "execute-remote", Response: &fakeplatform.PeerResponse{ID: "r1", Operation: "execute-remote", Status: 200, Payload: []byte("remote-ok")}}
	long := &fakeplatform.PeerMessage{Operation: "execute-response", Response: &fakeplatform.PeerResponse{ID: "l", Operation: "execute-response", Status: 200, Payload: bytes.Repeat([]byte("z"), 300)}}
	longAnswer := &fakeplatform.PeerMessage{Operation: "execute-remote", Response: &fakeplatform.PeerResponse{ID: "r2", Operation: "execute-remote", Status: 200, Payload: bytes.Repeat([]byte("s"), 300)}}
	longRemote := &fakeplatform.PeerMessage{Operation: "execute-remote", Remote: &fakeplatform.PeerRemoteRequest{ID: "r3", Operation: "execute-remote",
		Request: fakeplatform.PeerRemoteCommand{FAASFunction: "fn", URL: "http://x/z", Method: "POST", Payload: bytes.Repeat([]byte("r"), 300)}}}
	o := Observed{
		PeerEvents: []fakeplatform.PeerEvent{
			{Kind: "connected"},
			{Kind: "sent", Message: exec},
			{Kind: "received", Message: resp},
			{Kind: "received", Message: remote},
			{Kind: "sent", Message: answer},
			{Kind: "received", Message: long},
			{Kind: "sent", Message: longAnswer},
			{Kind: "received", Message: longRemote},
			{Kind: "proxy-connection", ConnectionID: "c1"},
			{Kind: "closed", By: "peer", Code: 1008, Text: "takeover"},
		},
		Local: []CallRecord{
			{Method: "POST", Target: "/echo?x=1", Status: 201, Body: "hi"},
			{Method: "POST", Target: "/big", Status: 200, Body: strings.Repeat("c", 300)},
		},
		Public: []CallRecord{
			{Method: "GET", Target: "/hello", Status: 200, Body: "hello from app"},
			{Method: "GET", Target: "/big", Status: 200, Body: strings.Repeat("p", 300)},
		},
	}
	want := "[peer]\nconnected\n" +
		"sent execute id=public-1 GET /hello\n" +
		"received execute-response id=public-1 status=200 payload=\"hello from app\"\n" +
		"received execute-remote id=r1 function=fn POST http://x/y payload=\"p\"\n" +
		"sent execute-remote id=r1 status=200 payload=\"remote-ok\"\n" +
		"received execute-response id=l status=200 payload=\"" + strings.Repeat("z", 200) + "\"\n" +
		"sent execute-remote id=r2 status=200 payload=\"" + strings.Repeat("s", 300) + "\"\n" +
		"received execute-remote id=r3 function=fn POST http://x/z payload=\"" + strings.Repeat("r", 300) + "\"\n" +
		"proxy-connection id=c1\n" +
		"closed by peer code=1008\n\n" +
		"[local]\nPOST /echo?x=1 -> 201 \"hi\"\nPOST /big -> 200 \"" + strings.Repeat("c", 200) + "\"\n\n" +
		"[public]\nGET /hello -> 200 \"hello from app\"\nGET /big -> 200 \"" + strings.Repeat("p", 200) + "\"\n"
	assert.Equal(t, want, RenderRequests(o, Normaliser{}))

	withRequest := RenderRequests(Observed{Requests: []fakeplatform.Request{mkReq("rest", "GET", "/a")}, Local: o.Local[:1]}, Normaliser{})
	assert.Equal(t, "euw1 GET /a\n  auth: none\n  acct: none\n  trace: no\n\n[local]\nPOST /echo?x=1 -> 201 \"hi\"\n", withRequest)
}

const readsSample = "euw1 GET /regions\n  auth: basic abc123\n\n" +
	"euw1 GET /regions/aws.euw1\n  auth: basic abc123\n\n" +
	"euw1 GET /runtimes\n  auth: basic abc123\n\n" +
	"euw1 GET /products/templates\n  auth: basic abc123\n\n" +
	"euw1 GET /projects/my-project\n  auth: basic abc123\n\n" +
	"euw1 GET /instances?filter=dev\n  auth: basic abc123\n\n" +
	"euw1 GET /instances/<uuid>\n  auth: basic abc123\n\n" +
	"euw1 GET /projects/my-project/instances/dev\n  auth: basic abc123\n\n" +
	"euw1 GET /instances/<uuid>/logs?limit=10\n  auth: basic abc123\n\n" +
	"euw1 GET /instances\n  auth: basic abc123\n\n" +
	"euw1 POST graphql:Regions\n  auth: basic abc123\n\n" +
	"euw1 DELETE /instances/<uuid>\n  auth: basic abc123\n\n" +
	"euw1 GET /secrets\n  auth: basic abc123\n\n" +
	"euw1 GET /applications\n  auth: basic abc123\n\n" +
	"euw1 GET /projects/my-project/instances\n  auth: basic abc123\n\n" +
	"gh GET /releases/latest\n  auth: none\n\n" +
	"[peer]\nconnected\n"

func TestReadsFilterRemovesTheNineRoutes(t *testing.T) {
	f := FilterReads(readsSample)
	assert.Equal(t, 1, f.GraphQL)
	assert.Equal(t, []string{
		"euw1 GET /regions", "euw1 GET /regions/aws.euw1", "euw1 GET /runtimes", "euw1 GET /products/templates",
		"euw1 GET /projects/my-project", "euw1 GET /instances?filter=dev", "euw1 GET /instances/<uuid>",
		"euw1 GET /projects/my-project/instances/dev", "euw1 GET /instances/<uuid>/logs?limit=10", "euw1 GET /instances",
	}, f.Reads)
	wantKept := "euw1 DELETE /instances/<uuid>\n  auth: basic abc123\n\n" +
		"euw1 GET /secrets\n  auth: basic abc123\n\n" +
		"euw1 GET /applications\n  auth: basic abc123\n\n" +
		"euw1 GET /projects/my-project/instances\n  auth: basic abc123\n\n" +
		"gh GET /releases/latest\n  auth: none\n\n" +
		"[peer]\nconnected\n"
	assert.Equal(t, wantKept, f.Kept)

	none := FilterReads("")
	assert.Empty(t, none.Kept)
	assert.Empty(t, none.Reads)
}

func TestExpectReadsCompare(t *testing.T) {
	a, b, c := "euw1 GET /a", "euw1 GET /b", "euw1 GET /c"
	assert.Empty(t, CompareReads([]string{a, b}, []string{b, a}), "order does not matter")
	assert.Empty(t, CompareReads([]string{a, a, b}, []string{a, b, a}), "order does not matter with repeats either")
	assert.Empty(t, CompareReads(nil, nil))

	missing := CompareReads([]string{a, b}, []string{a})
	assert.Contains(t, missing, "missing (expect-reads 1, made 0): "+b)
	assert.NotContains(t, missing, "extra")

	extra := CompareReads([]string{a}, []string{a, c})
	assert.Contains(t, extra, "extra (expect-reads 0, made 1): "+c)
	assert.NotContains(t, extra, "missing")

	extraRepeat := CompareReads([]string{a}, []string{a, a})
	assert.Contains(t, extraRepeat, "extra (expect-reads 1, made 2): "+a, "a read made twice must be listed twice")

	missingRepeat := CompareReads([]string{a, a, b}, []string{a, b})
	assert.Contains(t, missingRepeat, "missing (expect-reads 2, made 1): "+a)
	assert.NotContains(t, missingRepeat, b, "the entry that agrees is not named")

	both := CompareReads([]string{a, a}, []string{a, c, c})
	assert.Contains(t, both, "missing (expect-reads 2, made 1): "+a)
	assert.Contains(t, both, "extra (expect-reads 0, made 2): "+c)
}

func TestRepeatedReadsMustBeListedEachTime(t *testing.T) {
	read := "euw1 GET /instances/<uuid>/logs?limit=10"
	block := read + "\n  auth: none\n  acct: none\n  trace: no\n"
	requests := block + "\n" + block
	base, got := Transcript{Exit: "0", Requests: requests}, Result{Exit: "0", Requests: requests}
	opts := CompareOptions{NoGraphQL: true}

	once := mustScenario(t, "instance-polls", fmt.Sprintf("expect-reads: [%q]\n", read))
	fs := failuresOf(Compare(once, base, got, opts), "reads")
	require.Len(t, fs, 1, "two polls are two reads")
	assert.Contains(t, fs[0].Detail, "extra (expect-reads 1, made 2): "+read)

	twice := mustScenario(t, "instance-polls", fmt.Sprintf("expect-reads: [%q, %q]\n", read, read))
	assert.Empty(t, Compare(twice, base, got, opts))

	three := mustScenario(t, "instance-polls", fmt.Sprintf("expect-reads: [%q, %q, %q]\n", read, read, read))
	fs = failuresOf(Compare(three, base, got, opts), "reads")
	require.Len(t, fs, 1)
	assert.Contains(t, fs[0].Detail, "missing (expect-reads 3, made 2): "+read)
}

// gqlRequest is a GraphQL request of the baseline: the operation and its variables.
func gqlRequest(op, vars string) fakeplatform.Request {
	body := []byte(`{"query":"q","variables":` + vars + `}`)
	return mkReq("graphql", "POST", "", withBody("application/json", body), func(r *fakeplatform.Request) {
		r.Route, r.Rel = "POST graphql:"+op, ""
	})
}

func TestDerivedReadsFromGraphQL(t *testing.T) {
	gql := gqlRequest
	reqs := []fakeplatform.Request{
		gql("Regions", `{}`),
		gql("Region", `{"alias":"aws.euw1"}`),
		gql("Instances", `{}`),
		gql("InstancesFiltered", `{"service_name":"%dev%"}`),
		gql("InstanceByNames", `{"project_name":"my-project","instance_name":"dev"}`),
		gql("InstanceByID", `{"id":"abc"}`),
		gql("Runtimes", `{}`),
		gql("Project", `{"api_account_id":"a1","name":"my-project"}`),
		gql("Products", `{}`),
		gql("ProductVersion", `{"id":"p1"}`),
		gql("Logs", `{"instance_id":"abc","limit":50}`),
		gql("Logs", `{"instance_id":"abc","limit":50,"timestamp":"2026-10-10T10:00:00Z"}`),
		mkReq("rest", "GET", "/secrets"),
	}
	got := DerivedReads(reqs, NewNormaliser(Layout{}, "", 0, 0, nil, map[string]bool{"2026-10-10T10:00:00Z": true}))
	assert.Equal(t, []string{
		"euw1 GET /instances",
		"euw1 GET /instances/abc",
		"euw1 GET /instances/abc/logs?after=2026-10-10T10:00:00Z&limit=50",
		"euw1 GET /instances/abc/logs?limit=50",
		"euw1 GET /instances?filter=dev",
		"euw1 GET /products/templates",
		"euw1 GET /projects/my-project",
		"euw1 GET /projects/my-project/instances/dev",
		"euw1 GET /regions",
		"euw1 GET /regions/aws.euw1",
		"euw1 GET /runtimes",
	}, got)
}

func TestDerivedReadsKeepOneEntryPerOperation(t *testing.T) {
	n := Normaliser{}
	polls := gqlRequest("Logs", `{"instance_id":"abc","limit":50}`)
	assert.Equal(t, []string{"euw1 GET /instances/abc/logs?limit=50", "euw1 GET /instances/abc/logs?limit=50"},
		DerivedReads([]fakeplatform.Request{polls, polls}, n), "two Logs operations are two reads")

	region, regions := gqlRequest("Region", `{"alias":"aws.euw1"}`), gqlRequest("Regions", `{}`)
	assert.Equal(t, []string{"euw1 GET /regions", "euw1 GET /regions/aws.euw1", "euw1 GET /regions/aws.euw1"},
		DerivedReads([]fakeplatform.Request{region, regions, region}, n))

	products, version := gqlRequest("Products", `{}`), gqlRequest("ProductVersion", `{"id":"p1"}`)
	templates := "euw1 GET /products/templates"
	assert.Equal(t, []string{templates}, DerivedReads([]fakeplatform.Request{products, version}, n), "ProductVersion folds into the Products read")
	assert.Equal(t, []string{templates}, DerivedReads([]fakeplatform.Request{version, version}, n), "alone, ProductVersion stands for one read of the templates")
	assert.Equal(t, []string{templates, templates}, DerivedReads([]fakeplatform.Request{products, version, products}, n))
	assert.Empty(t, DerivedReads(nil, n))
}

func TestRelaxGitHubCredentialsOnlyTouchesGitHubBlocks(t *testing.T) {
	in := "euw1 GET /secrets\n  auth: basic abc123\n  acct: abc123\n  trace: yes\n\n" +
		"gh GET /releases/latest\n  auth: basic abc123\n  acct: abc123\n  trace: yes\n  type: text/plain\n\n" +
		"[peer]\nconnected\n"
	want := "euw1 GET /secrets\n  auth: basic abc123\n  acct: abc123\n  trace: yes\n\n" +
		"gh GET /releases/latest\n  type: text/plain\n\n" +
		"[peer]\nconnected\n"
	assert.Equal(t, want, RelaxGitHubCredentials(in))
	assert.Equal(t, "euw1 GET /a\n  auth: none\n", RelaxGitHubCredentials("euw1 GET /a\n  auth: none\n"))
}
