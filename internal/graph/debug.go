package graph

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// debugBodyFile is the path set via TEAMS_TUI_DEBUG_BODIES. When set, every
// fetched message's raw body (contentType + content) is appended to it. This is
// a diagnostic aid for understanding exactly how Graph stores a message (e.g.
// how Teams canonicalizes a sent code block), with no effect when unset.
var (
	debugBodyOnce sync.Once
	debugBodyPath string
)

func debugDumpMessages(msgs []Message) {
	debugBodyOnce.Do(func() {
		debugBodyPath = os.Getenv("TEAMS_TUI_DEBUG_BODIES")
	})
	if debugBodyPath == "" || len(msgs) == 0 {
		return
	}
	f, err := os.OpenFile(debugBodyPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	for _, m := range msgs {
		fmt.Fprintf(f, "=== %s id=%s type=%q contentType=%q ===\n%s\n\n",
			time.Now().Format(time.RFC3339), m.ID, m.MessageType, m.Body.ContentType, m.Body.Content)
	}
}
