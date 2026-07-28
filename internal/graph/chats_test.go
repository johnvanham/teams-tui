package graph

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jvh/teams-tui/internal/auth"
)

// chatPage renders a listResponse-shaped payload holding n chats named
// "<prefix>-<i>", optionally followed by nextLink.
func chatPage(prefix string, n int, nextLink string) string {
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, fmt.Sprintf(`{"id":%q}`, fmt.Sprintf("%s-%d", prefix, i)))
	}
	body := `{"value":[` + strings.Join(ids, ",") + `]`
	if nextLink != "" {
		body += fmt.Sprintf(`,"@odata.nextLink":%q`, nextLink)
	}
	return body + "}"
}

// chatPageIDs renders a listResponse-shaped payload holding the given chat IDs
// verbatim, so a test can make two pages overlap.
func chatPageIDs(ids []string, nextLink string) string {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, fmt.Sprintf(`{"id":%q}`, id))
	}
	body := `{"value":[` + strings.Join(quoted, ",") + `]`
	if nextLink != "" {
		body += fmt.Sprintf(`,"@odata.nextLink":%q`, nextLink)
	}
	return body + "}"
}

// chatServer serves the given page bodies in order, calling nextLink on each
// one with the server's own URL so the client can follow them. It records the
// request paths it served.
func chatServer(t *testing.T, pages ...func(base string) string) (*Client, *[]string) {
	t.Helper()
	var paths []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		i := len(paths) - 1
		if i >= len(pages) {
			t.Errorf("unexpected extra request for %s", r.URL.RequestURI())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body := pages[i](srv.URL)
		if body == "" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"boom","message":"nope"}}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	tok := &auth.Token{AccessToken: "token", Expiry: time.Now().Add(time.Hour)}
	return NewClient(srv.URL, auth.NewTokenSource(nil, nil, tok)), &paths
}

// Graph hands out chats one page at a time; ListChats must follow
// @odata.nextLink so the sidebar isn't capped at a single page.
func TestListChatsFollowsNextLink(t *testing.T) {
	c, paths := chatServer(t,
		func(base string) string { return chatPage("p1", 50, base+"/me/chats?$skiptoken=abc") },
		func(base string) string { return chatPage("p2", 30, "") },
	)

	chats, err := c.ListChats(context.Background(), MaxChatPageSize, 200)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(chats) != 80 {
		t.Errorf("len(chats) = %d, want 80", len(chats))
	}
	if len(*paths) != 2 {
		t.Fatalf("requests = %d (%v), want 2", len(*paths), *paths)
	}
	if !strings.Contains((*paths)[0], "%24top=50") {
		t.Errorf("first request %q does not ask for a full page", (*paths)[0])
	}
	if !strings.Contains((*paths)[1], "skiptoken=abc") {
		t.Errorf("second request %q did not follow the nextLink", (*paths)[1])
	}
}

// Paging stops as soon as max chats have been collected, and the result is
// trimmed to exactly max.
func TestListChatsStopsAtMax(t *testing.T) {
	c, paths := chatServer(t,
		func(base string) string { return chatPage("p1", 50, base+"/me/chats?$skiptoken=abc") },
		func(base string) string { return chatPage("p2", 50, base+"/me/chats?$skiptoken=def") },
	)

	chats, err := c.ListChats(context.Background(), MaxChatPageSize, 60)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(chats) != 60 {
		t.Errorf("len(chats) = %d, want 60", len(chats))
	}
	if len(*paths) != 2 {
		t.Errorf("requests = %d, want 2 (must not fetch a third page)", len(*paths))
	}
}

// A later page failing keeps the chats already gathered: they are the most
// recently active ones, so a partial sidebar beats an empty one.
func TestListChatsPartialOnLaterPageError(t *testing.T) {
	c, _ := chatServer(t,
		func(base string) string { return chatPage("p1", 50, base+"/me/chats?$skiptoken=abc") },
		func(base string) string { return "" }, // 500
	)

	chats, err := c.ListChats(context.Background(), MaxChatPageSize, 200)
	if err != nil {
		t.Fatalf("ListChats() error = %v, want nil (partial results)", err)
	}
	if len(chats) != 50 {
		t.Errorf("len(chats) = %d, want 50", len(chats))
	}
}

// Graph pages this collection with a skiptoken over a mutable sort key, so a
// chat that receives a message between two page requests can be handed back on
// both pages. Returning it twice put the same conversation in the sidebar
// twice, so ListChats de-duplicates by ID.
func TestListChatsDedupesAcrossPages(t *testing.T) {
	c, _ := chatServer(t,
		func(base string) string {
			return chatPageIDs([]string{"a", "b", "c"}, base+"/me/chats?$skiptoken=abc")
		},
		// "b" got a message mid-paging and reappears alongside genuinely new chats.
		func(base string) string { return chatPageIDs([]string{"b", "d"}, "") },
	)

	chats, err := c.ListChats(context.Background(), MaxChatPageSize, 200)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}

	var got []string
	for _, chat := range chats {
		got = append(got, chat.ID)
	}
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

// The max cap counts distinct chats: duplicates must not eat into the budget
// (and so silently shorten the sidebar).
func TestListChatsMaxCountsDistinctChats(t *testing.T) {
	c, _ := chatServer(t,
		func(base string) string {
			return chatPageIDs([]string{"a", "b"}, base+"/me/chats?$skiptoken=abc")
		},
		func(base string) string {
			return chatPageIDs([]string{"b", "c"}, base+"/me/chats?$skiptoken=def")
		},
		func(base string) string { return chatPageIDs([]string{"d"}, "") },
	)

	chats, err := c.ListChats(context.Background(), MaxChatPageSize, 4)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}

	var got []string
	for _, chat := range chats {
		got = append(got, chat.ID)
	}
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v (max must count distinct chats)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v (max must count distinct chats)", got, want)
		}
	}
}

// A failure on the very first page is a real error: there is nothing to show.
func TestListChatsFirstPageErrorFails(t *testing.T) {
	c, _ := chatServer(t, func(base string) string { return "" })

	if _, err := c.ListChats(context.Background(), MaxChatPageSize, 200); err == nil {
		t.Error("ListChats() error = nil, want an error")
	}
}
