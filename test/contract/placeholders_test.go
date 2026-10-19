//go:build !windows

package contract

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandRunnerPlaceholders(t *testing.T) {
	l := testLayout()
	raw := "sandbox: ${SANDBOX}\nhome: ${HOME}\nwork: ${WORK}\ntmp: ${TMP}\noutside: ${OUTSIDE}\nbin: ${BIN}\n" +
		"app: ${TESTAPP}\np1: ${PORT:x}\np2: ${PORT:y}\np3: ${PORT:x}\n" +
		"fake: ${FAKE} ${FAKE_HOST} ${OTHER} ${OTHER_HOST} ${OTHER_SAME_HOST} ${ACCOUNT} ${SECRET}\nunknown: ${NOPE}\n"

	ports, err := AllocatePorts(PortNames(raw), nil)
	require.NoError(t, err)
	t.Cleanup(ports.Close)
	out := ExpandRunner(raw, l, "/tmp/x/testapp", ports)

	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		k, v, _ := strings.Cut(line, ": ")
		got[k] = v
	}
	assert.Equal(t, "/tmp/x/s", got["sandbox"])
	assert.Equal(t, "/tmp/x/s/home", got["home"])
	assert.Equal(t, "/tmp/x/s/work", got["work"])
	assert.Equal(t, "/tmp/x/s/tmp", got["tmp"])
	assert.Equal(t, "/tmp/x/s/outside", got["outside"])
	assert.Equal(t, "/tmp/x/s/bin", got["bin"])
	assert.Equal(t, "/tmp/x/testapp", got["app"])
	assert.Equal(t, got["p1"], got["p3"], "a repeated name is one port")
	assert.NotEqual(t, got["p1"], got["p2"])
	for _, p := range []string{got["p1"], got["p2"]} {
		n, err := strconv.Atoi(p)
		require.NoError(t, err)
		assert.Positive(t, n)
	}
	assert.Equal(t, "${FAKE} ${FAKE_HOST} ${OTHER} ${OTHER_HOST} ${OTHER_SAME_HOST} ${ACCOUNT} ${SECRET}", got["fake"])
	assert.Equal(t, "${NOPE}", got["unknown"])
}

func TestPortNamesAreDistinctInOrderOfAppearance(t *testing.T) {
	assert.Equal(t, []string{"b", "a"}, PortNames("${PORT:b} x ${PORT:a} ${PORT:b}"))
	assert.Empty(t, PortNames("no ports ${FAKE}"))
}

func TestHeldPortsStayBoundUntilClosed(t *testing.T) {
	ports, err := AllocatePorts([]string{"held", "loose"}, []string{"held"})
	require.NoError(t, err)
	held := ports.Port("held")
	require.Positive(t, held)
	require.Positive(t, ports.Port("loose"))

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(held)))
	require.NoError(t, err, "the runner holds the port open")
	_ = conn.Close()

	ports.Close()
	_, err = (&net.Dialer{}).DialContext(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(held)))
	assert.Error(t, err, "closing the set releases the held ports")
}

func TestHeldNameWithoutAPlaceholderIsStillAllocated(t *testing.T) {
	ports, err := AllocatePorts(nil, []string{"only-held"})
	require.NoError(t, err)
	t.Cleanup(ports.Close)
	assert.Positive(t, ports.Port("only-held"))
	assert.Zero(t, ports.Port("unknown"))
}
