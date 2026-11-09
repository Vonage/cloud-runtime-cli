//go:build !windows

package contract

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

// segInstances is the path segment of the instance routes.
const segInstances = "instances"

// Filtered is a requests section after the reads filter.
type Filtered struct {
	Kept    string   // the section without the filtered blocks
	Reads   []string // the first line of every removed read GET, in order
	GraphQL int      // the number of removed GraphQL blocks
}

// splitBlocks cuts a requests section into its blocks; each keeps its final newline.
func splitBlocks(section string) []string {
	if section == "" {
		return nil
	}
	parts := strings.Split(section, "\n\n")
	for i := 0; i < len(parts)-1; i++ {
		parts[i] += "\n"
	}
	return parts
}

func firstLine(block string) string {
	line, _, _ := strings.Cut(block, "\n")
	return line
}

// FilterReads removes from a requests section every GraphQL block and every
// GET of one of the read routes that the baseline never calls.
func FilterReads(section string) Filtered {
	var f Filtered
	var kept []string
	for _, block := range splitBlocks(section) {
		head := firstLine(block)
		fields := strings.SplitN(head, " ", 3)
		if len(fields) == 3 && fields[0] != laneGitHub && fields[0] != laneOther && !strings.HasPrefix(head, "[") {
			switch {
			case fields[1] == "POST" && strings.HasPrefix(fields[2], "graphql:"):
				f.GraphQL++
				continue
			case fields[1] == "GET" && isReadRoute(fields[2]):
				f.Reads = append(f.Reads, head)
				continue
			}
		}
		kept = append(kept, block)
	}
	f.Kept = strings.Join(kept, "\n")
	return f
}

// isReadRoute reports whether target (a rel with its query) is one of the
// read routes: the nine that replace the GraphQL queries, and the build status.
func isReadRoute(target string) bool {
	rel, _, _ := strings.Cut(target, "?")
	segs := strings.Split(strings.TrimPrefix(rel, "/"), "/")
	switch {
	case rel == "/regions", rel == "/runtimes", rel == "/products/templates", rel == "/instances":
		return true
	case len(segs) == 2 && segs[1] != "":
		return segs[0] == "regions" || segs[0] == "projects" || segs[0] == segInstances
	case len(segs) == 3:
		return segs[0] == segInstances && segs[2] == "logs" && segs[1] != ""
	case len(segs) == 4 && segs[0] == "packages":
		return segs[1] != "" && segs[2] == "build" && segs[3] == "status"
	case len(segs) == 4:
		return segs[0] == "projects" && segs[2] == segInstances && segs[1] != "" && segs[3] != ""
	}
	return false
}

// CompareReads compares the reads a binary made with the scenario's
// expect-reads as sorted lists: the order does not matter, but a read made twice
// must be listed twice. It returns "" when they agree, else a text that names
// each missing and extra entry with the count on both sides.
func CompareReads(want, got []string) string {
	w, g := countEntries(want), countEntries(got)
	entries := make([]string, 0, len(w)+len(g))
	for e := range w {
		entries = append(entries, e)
	}
	for e := range g {
		if _, listed := w[e]; !listed {
			entries = append(entries, e)
		}
	}
	sort.Strings(entries)
	var missing, extra strings.Builder
	for _, e := range entries {
		switch {
		case w[e] > g[e]:
			fmt.Fprintf(&missing, "  missing (expect-reads %d, made %d): %s\n", w[e], g[e], e)
		case g[e] > w[e]:
			fmt.Fprintf(&extra, "  extra (expect-reads %d, made %d): %s\n", w[e], g[e], e)
		}
	}
	if missing.Len() == 0 && extra.Len() == 0 {
		return ""
	}
	return "the reads of the binary differ from expect-reads (compared as sorted lists)\n" + missing.String() + extra.String()
}

func countEntries(list []string) map[string]int {
	out := make(map[string]int, len(list))
	for _, e := range list {
		out[e]++
	}
	return out
}

// graphqlVars are the variables of a GraphQL request body.
type graphqlVars map[string]json.RawMessage

func (v graphqlVars) text(name string) string {
	var s string
	if json.Unmarshal(v[name], &s) != nil {
		return ""
	}
	return s
}

func (v graphqlVars) number(name string) int {
	var n int
	if json.Unmarshal(v[name], &n) != nil {
		return 0
	}
	return n
}

// DerivedReads lists the reads that the GraphQL operations of a baseline run
// stand for, sorted, one entry per operation: a read the baseline made twice is
// listed twice. A ProductVersion operation folds into the Products read of the
// same run; it has an entry of its own (one, however many there are) only when
// the run made no Products operation. It is a hint for whoever writes a
// scenario's expect-reads.
func DerivedReads(reqs []fakeplatform.Request, n Normaliser) []string {
	var out, versions []string
	products := false
	for _, r := range reqs {
		op, ok := strings.CutPrefix(r.Route, "POST graphql:")
		if !ok || r.Kind != kindGraphQL {
			continue
		}
		var body struct {
			Variables graphqlVars `json:"variables"`
		}
		_ = json.Unmarshal(r.Body, &body)
		rel := derivedRel(op, body.Variables)
		if rel == "" {
			continue
		}
		entry := n.Text(laneOf(r) + " GET " + rel)
		switch op {
		case "ProductVersion":
			versions = append(versions, entry)
		case "Products":
			products = true
			out = append(out, entry)
		default:
			out = append(out, entry)
		}
	}
	if !products && len(versions) > 0 {
		out = append(out, versions[0])
	}
	sort.Strings(out)
	return out
}

// derivedRel is the read that a GraphQL operation stands for.
func derivedRel(op string, v graphqlVars) string {
	switch op {
	case "Regions":
		return "/regions"
	case "Region":
		return "/regions/" + v.text("alias")
	case "Instances":
		return "/instances"
	case "InstancesFiltered":
		return "/instances?" + canonicalQuery(url.Values{"filter": {strings.Trim(v.text("service_name"), "%")}}.Encode())
	case "InstanceByNames":
		return "/projects/" + v.text("project_name") + "/instances/" + v.text("instance_name")
	case "InstanceByID":
		return "/instances/" + v.text("id")
	case "Runtimes":
		return "/runtimes"
	case "Project":
		return "/projects/" + v.text("name")
	case "Products", "ProductVersion":
		return "/products/templates"
	case "Logs":
		q := url.Values{"limit": {strconv.Itoa(v.number("limit"))}}
		if ts := v.text("timestamp"); ts != "" {
			q.Set("after", ts)
		}
		return "/instances/" + v.text("instance_id") + "/logs?" + canonicalQuery(q.Encode())
	}
	return ""
}

// RelaxGitHubCredentials removes the auth, acct and trace lines from the gh
// blocks of a requests section: a release client of an older version sent
// credentials to the release host, and a newer one does not.
func RelaxGitHubCredentials(section string) string {
	blocks := splitBlocks(section)
	for i, block := range blocks {
		if !strings.HasPrefix(block, laneGitHub+" ") {
			continue
		}
		var kept []string
		for _, line := range strings.SplitAfter(block, "\n") {
			if strings.HasPrefix(line, "  auth:") || strings.HasPrefix(line, "  acct:") || strings.HasPrefix(line, "  trace:") {
				continue
			}
			kept = append(kept, line)
		}
		blocks[i] = strings.Join(kept, "")
	}
	return strings.Join(blocks, "\n")
}

// RelaxWebsocketTrace removes the trace line from the websocket blocks of a requests section: the build watch and the
// debug tunnel and proxy are dialled with a fresh trace id now, where the baseline sent none. Every other line of those
// blocks, and every other block, stays as it is.
func RelaxWebsocketTrace(section string) string {
	blocks := splitBlocks(section)
	for i, block := range blocks {
		if !isWebsocketBlock(block) {
			continue
		}
		var kept []string
		for _, line := range strings.SplitAfter(block, "\n") {
			if !isTraceMarker(line) {
				kept = append(kept, line)
			}
		}
		blocks[i] = strings.Join(kept, "")
	}
	return strings.Join(blocks, "\n")
}

// isWebsocketBlock reports whether the head of a request block, "<lane> WS <target>", is a websocket handshake.
func isWebsocketBlock(block string) bool {
	fields := strings.SplitN(firstLine(block), " ", 3)
	return len(fields) == 3 && fields[1] == "WS"
}

// isTraceMarker reports whether a line is the trace line of a request block with one of its two values.
func isTraceMarker(line string) bool {
	switch strings.TrimSuffix(line, "\n") {
	case "  trace: yes", "  trace: no":
		return true
	}
	return false
}

// describeReads is a one-line form of a list of reads for messages.
func describeReads(reads []string) string {
	if len(reads) == 0 {
		return "no reads"
	}
	return fmt.Sprintf("%d read(s): %s", len(reads), strings.Join(reads, "; "))
}
