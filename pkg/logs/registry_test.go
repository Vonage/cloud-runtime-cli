package logs

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistry_AssignsStableShortIDs(t *testing.T) {
	r := NewRegistry()

	a := r.Ensure("vcr-app-7d4f9c8b6-x2k9p")
	b := r.Ensure("vcr-app-7d4f9c8b6-qq111")
	again := r.Ensure("vcr-app-7d4f9c8b6-x2k9p")

	require.Equal(t, "r1", a.ShortID)
	require.Equal(t, "r2", b.ShortID)
	require.Equal(t, "r1", again.ShortID, "same hostname keeps its id")
	require.Equal(t, 2, r.Len())
	require.NotEqual(t, a.ColorIndex, b.ColorIndex)
}

func TestRegistry_EmptyHostnameIsNotRegistered(t *testing.T) {
	r := NewRegistry()
	got := r.Ensure("")
	require.Equal(t, "", got.ShortID)
	require.Equal(t, 0, r.Len())
}

func TestRegistry_ListIsFirstSeenOrder(t *testing.T) {
	r := NewRegistry()
	r.Ensure("host-b")
	r.Ensure("host-a")
	list := r.List()
	require.Len(t, list, 2)
	require.Equal(t, "host-b", list[0].Hostname)
	require.Equal(t, "host-a", list[1].Hostname)
}

func TestRegistry_ResolveByShortIDOrHostnameSubstring(t *testing.T) {
	r := NewRegistry()
	r.Ensure("vcr-app-abc-12345")

	got, ok := r.Resolve("r1")
	require.True(t, ok)
	require.Equal(t, "vcr-app-abc-12345", got.Hostname)

	got, ok = r.Resolve("12345")
	require.True(t, ok)
	require.Equal(t, "r1", got.ShortID)

	_, ok = r.Resolve("nope")
	require.False(t, ok)
}

func TestRegistry_ResolveExactHostnameBeatsEarlierSubstringMatch(t *testing.T) {
	r := NewRegistry()
	// xabcx is registered FIRST and contains "abc" as a substring, so a
	// substring-first Resolve would return it. The exact-hostname tier must win.
	r.Ensure("xabcx")
	r.Ensure("abc")

	got, ok := r.Resolve("abc")
	require.True(t, ok)
	require.Equal(t, "abc", got.Hostname, "exact hostname match must beat an earlier-registered substring match")
	require.Equal(t, "r2", got.ShortID)
}

func TestRegistry_ResolveEmptyTokenIsNotFound(t *testing.T) {
	r := NewRegistry()
	r.Ensure("vcr-app-abc-12345")

	_, ok := r.Resolve("")
	require.False(t, ok, "an empty token must never resolve")
}

func TestRegistry_ConcurrentEnsureAndList(t *testing.T) {
	r := NewRegistry()

	const writers = 8
	const perWriter = 50

	var wg sync.WaitGroup
	wg.Add(writers * 2)
	for w := 0; w < writers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				r.Ensure(fmt.Sprintf("host-%d", i%4))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_ = r.List()
				_ = r.Len()
				_, _ = r.Resolve("r1")
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 4, r.Len(), "only distinct hostnames are registered")
	total := 0
	for _, rep := range r.List() {
		total += rep.Count
	}
	require.Equal(t, writers*perWriter, total, "every Ensure is counted exactly once")

	// A racy len(r.order) read while assigning ShortID would hand the same short
	// id to two replicas; assert every id is distinct and accounted for.
	list := r.List()
	seen := map[string]bool{}
	for _, rep := range list {
		require.False(t, seen[rep.ShortID], "duplicate ShortID %q assigned", rep.ShortID)
		seen[rep.ShortID] = true
	}
	require.Len(t, seen, r.Len(), "one unique ShortID per registered replica")
	require.Len(t, list, r.Len())
}
