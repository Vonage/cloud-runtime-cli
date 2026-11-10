package term

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const updateNotice = "Update available 2.6.1 → 2.7.0 · run vcr upgrade"

func TestNoticeIsOneDimLineOnStderr(t *testing.T) {
	streams, stdout, stderr, _ := newRich(t, Rich, 80)

	streams.Notice(updateNotice)

	assert.Equal(t, "\n"+updateNotice+"\n", stderr.Screen())
	assert.Equal(t, "\n"+streams.Style().Gray(updateNotice)+"\n", stderr.Raw())
	assert.Empty(t, stdout.Raw())
}

func TestNoticeInStatic(t *testing.T) {
	streams, stdout, stderr, _ := newRich(t, Static, 80)

	streams.Notice(updateNotice)

	assert.Equal(t, "\n"+updateNotice+"\n", stderr.Screen())
	assert.Empty(t, stdout.Raw())
}

func TestNoticeGoesThroughTheLockedWriter(t *testing.T) {
	d := newDrawn(t)

	d.streams.Notice(updateNotice)

	assert.Equal(t, "\n"+updateNotice+"\n"+frame(300*time.Millisecond)+" "+uploadLabel, d.screen.Screen())
}
