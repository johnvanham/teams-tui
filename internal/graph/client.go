// Package graph is a small Microsoft Graph v1.0 client scoped to the features
// teams-tui needs: listing chats, reading and sending chat messages, resolving
// the signed-in user, and reading upcoming calendar events for meeting alerts.
package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jvh/teams-tui/internal/auth"
)

// Client talks to Microsoft Graph using a TokenSource for authorization.
type Client struct {
	baseURL string
	tokens  *auth.TokenSource
	http    *http.Client
}

// NewClient constructs a Graph client.
func NewClient(baseURL string, tokens *auth.TokenSource) *Client {
	return &Client{
		baseURL: baseURL,
		tokens:  tokens,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError carries an HTTP status and Graph error payload.
type APIError struct {
	Status int
	Code   string
	Msg    string
	Method string
	Path   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s -> %d %s: %s", e.Method, e.Path, e.Status, e.Code, e.Msg)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, extraHeaders map[string]string, out any) error {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}

	endpoint := path
	if len(path) == 0 || path[0] == '/' {
		endpoint = c.baseURL + path
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseAPIError(resp, method, path)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

func parseAPIError(resp *http.Response, method, path string) error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(data, &payload)
	// Strip query strings from the path so the status line stays readable.
	if i := len(path); i > 0 {
		if q := indexByte(path, '?'); q >= 0 {
			path = path[:q]
		}
	}
	return &APIError{
		Status: resp.StatusCode,
		Code:   payload.Error.Code,
		Msg:    payload.Error.Message,
		Method: method,
		Path:   path,
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

type listResponse[T any] struct {
	Value    []T    `json:"value"`
	NextLink string `json:"@odata.nextLink"`
}

// Me returns the signed-in user's profile.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, http.MethodGet, "/me", nil, nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// MyPresence returns the signed-in user's own presence (GET /me/presence).
func (c *Client) MyPresence(ctx context.Context) (*Presence, error) {
	var p Presence
	if err := c.do(ctx, http.MethodGet, "/me/presence", nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SetPresence creates or refreshes the app's presence session for the user,
// keeping the chosen status active. sessionId must be the app's client ID.
// Requires Presence.ReadWrite. expiration is the session lifetime (5–240 min).
func (c *Client) SetPresence(ctx context.Context, userID, sessionID, availability, activity string, expiration time.Duration) error {
	payload := map[string]any{
		"sessionId":          sessionID,
		"availability":       availability,
		"activity":           activity,
		"expirationDuration": isoDuration(expiration),
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/users/%s/presence/setPresence", url.PathEscape(userID))
	return c.do(ctx, http.MethodPost, path, bytes.NewReader(buf), nil, nil)
}

// SetUserPreferredPresence sets the user's preferred availability/activity,
// which persists across sessions (like clicking your avatar in Teams).
// Requires Presence.ReadWrite.
func (c *Client) SetUserPreferredPresence(ctx context.Context, userID, availability, activity string, expiration time.Duration) error {
	payload := map[string]any{
		"availability": availability,
		"activity":     activity,
	}
	if expiration > 0 {
		payload["expirationDuration"] = isoDuration(expiration)
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/users/%s/presence/setUserPreferredPresence", url.PathEscape(userID))
	return c.do(ctx, http.MethodPost, path, bytes.NewReader(buf), nil, nil)
}

// ClearPresence ends the app's presence session (POST .../clearPresence).
func (c *Client) ClearPresence(ctx context.Context, userID, sessionID string) error {
	payload := map[string]any{"sessionId": sessionID}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/users/%s/presence/clearPresence", url.PathEscape(userID))
	return c.do(ctx, http.MethodPost, path, bytes.NewReader(buf), nil, nil)
}

// isoDuration renders a duration as an ISO-8601 minutes value (e.g. "PT5M").
func isoDuration(d time.Duration) string {
	mins := int(d.Minutes())
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf("PT%dM", mins)
}

// ListChats returns the user's chats, expanding members and last message
// preview so the chat list can render names and previews without extra calls.
func (c *Client) ListChats(ctx context.Context, top int) ([]Chat, error) {
	if top <= 0 {
		top = 50
	}
	q := url.Values{}
	q.Set("$top", fmt.Sprintf("%d", top))
	q.Set("$expand", "members,lastMessagePreview")
	q.Set("$orderby", "lastMessagePreview/createdDateTime desc")

	var resp listResponse[Chat]
	if err := c.do(ctx, http.MethodGet, "/me/chats?"+q.Encode(), nil, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}

// ListPeople returns contacts relevant to the signed-in user via GET
// /me/people, powering the "start new chat" people picker. When search is
// non-empty it adds a $search filter (Graph requires the value to be wrapped in
// double quotes) and the ConsistencyLevel: eventual header that $search
// requires. Results are capped at 25 and trimmed to the fields the picker
// needs.
func (c *Client) ListPeople(ctx context.Context, search string) ([]Person, error) {
	q := url.Values{}
	q.Set("$top", "25")
	q.Set("$select", "id,displayName,userPrincipalName,scoredEmailAddresses")

	path := "/me/people?" + q.Encode()
	var headers map[string]string
	if search != "" {
		// $search must carry a double-quoted value (e.g. $search="bob"); the
		// quotes are part of the value, so append it after url.Values encoding.
		path += "&$search=" + url.QueryEscape(`"`+search+`"`)
		headers = map[string]string{"ConsistencyLevel": "eventual"}
	}

	var resp listResponse[Person]
	if err := c.do(ctx, http.MethodGet, path, nil, headers, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}

// CreateOneOnOneChat creates a 1:1 chat between the signed-in user and another
// user via POST /chats, returning the created chat so the caller can open it.
// The user@odata.bind references are built from the client's baseURL so custom
// Graph endpoints are respected.
func (c *Client) CreateOneOnOneChat(ctx context.Context, myUserID, otherUserID string) (*Chat, error) {
	member := func(userID string) map[string]any {
		return map[string]any{
			"@odata.type":     "#microsoft.graph.aadUserConversationMember",
			"roles":           []string{"owner"},
			"user@odata.bind": c.baseURL + "/users('" + userID + "')",
		}
	}
	payload := map[string]any{
		"chatType": "oneOnOne",
		"members": []map[string]any{
			member(myUserID),
			member(otherUserID),
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var chat Chat
	if err := c.do(ctx, http.MethodPost, "/chats", bytes.NewReader(buf), nil, &chat); err != nil {
		return nil, err
	}
	return &chat, nil
}

// GetPresences fetches presence for up to 650 users in one batch via
// POST /communications/getPresencesByUserId. Requires the Presence.Read.All
// delegated permission. Returns a map keyed by user ID.
func (c *Client) GetPresences(ctx context.Context, userIDs []string) (map[string]Presence, error) {
	if len(userIDs) == 0 {
		return map[string]Presence{}, nil
	}
	payload := map[string]any{"ids": userIDs}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var resp listResponse[Presence]
	if err := c.do(ctx, http.MethodPost, "/communications/getPresencesByUserId",
		bytes.NewReader(buf), nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]Presence, len(resp.Value))
	for _, p := range resp.Value {
		out[p.ID] = p
	}
	return out, nil
}

// ListMessages returns recent messages in a chat, newest first per Graph; the
// caller is responsible for ordering for display.
func (c *Client) ListMessages(ctx context.Context, chatID string, top int) ([]Message, error) {
	msgs, _, err := c.ListMessagesPage(ctx, chatID, top)
	return msgs, err
}

// ListMessagesPage fetches one page of a chat's messages (newest first) and
// returns the @odata.nextLink for fetching older messages, or "" when there are
// no more.
func (c *Client) ListMessagesPage(ctx context.Context, chatID string, top int) ([]Message, string, error) {
	if top <= 0 {
		top = 30
	}
	q := url.Values{}
	q.Set("$top", fmt.Sprintf("%d", top))

	path := fmt.Sprintf("/me/chats/%s/messages?%s", url.PathEscape(chatID), q.Encode())
	var resp listResponse[Message]
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &resp); err != nil {
		return nil, "", err
	}
	return resp.Value, resp.NextLink, nil
}

// FollowMessagesPage fetches the next page of messages from an absolute
// @odata.nextLink URL, returning the messages and the subsequent nextLink.
func (c *Client) FollowMessagesPage(ctx context.Context, nextLink string) ([]Message, string, error) {
	if nextLink == "" {
		return nil, "", nil
	}
	var resp listResponse[Message]
	// nextLink is an absolute URL; c.do uses it directly when it isn't a path.
	if err := c.do(ctx, http.MethodGet, nextLink, nil, nil, &resp); err != nil {
		return nil, "", err
	}
	return resp.Value, resp.NextLink, nil
}

// ListMessagesSince fetches only messages created or changed after the given
// time, using the server-side $filter on lastModifiedDateTime. This returns
// tiny payloads (usually empty), enabling frequent near-real-time polling of an
// open chat without re-listing the whole conversation. Requires $orderby on the
// same property per Graph's filtering rules.
func (c *Client) ListMessagesSince(ctx context.Context, chatID string, since time.Time) ([]Message, error) {
	q := url.Values{}
	q.Set("$top", "50")
	q.Set("$orderby", "lastModifiedDateTime desc")
	q.Set("$filter", fmt.Sprintf("lastModifiedDateTime gt %s",
		since.UTC().Format("2006-01-02T15:04:05.000Z")))

	path := fmt.Sprintf("/me/chats/%s/messages?%s", url.PathEscape(chatID), q.Encode())
	var resp listResponse[Message]
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}

// SendMessage posts a plaintext message to a chat and returns the created
// message.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) (*Message, error) {
	payload := map[string]any{
		"body": map[string]string{
			"contentType": "text",
			"content":     text,
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/me/chats/%s/messages", url.PathEscape(chatID))
	var msg Message
	if err := c.do(ctx, http.MethodPost, path, bytes.NewReader(buf), nil, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditMessage replaces the plaintext body of an existing chat message via
// PATCH /chats/{chat-id}/messages/{message-id}. Note the path uses /chats/...
// (not /me/chats/...) because Graph's message PATCH endpoint lives under
// /chats/{id}/messages/{id}. Graph returns 204 No Content with no body on a
// successful edit, so there is nothing to decode; on success we synthesize a
// minimal Message carrying the new body so the caller has something to render.
func (c *Client) EditMessage(ctx context.Context, chatID, messageID, text string) (*Message, error) {
	payload := map[string]any{
		"body": map[string]string{
			"contentType": "text",
			"content":     text,
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/chats/%s/messages/%s",
		url.PathEscape(chatID), url.PathEscape(messageID))
	// Pass nil as out: the PATCH responds 204 No Content, so there is no body
	// to decode. A nil error means the edit succeeded.
	if err := c.do(ctx, http.MethodPatch, path, bytes.NewReader(buf), nil, nil); err != nil {
		return nil, err
	}
	return &Message{ID: messageID, Body: MessageBody{ContentType: "text", Content: text}}, nil
}

// SendImageMessage posts a chat message with an inline image. Teams carries
// inline images as "hosted content": the image bytes are base64-encoded into a
// hostedContents entry tagged with a temporary id, and the HTML body references
// them via <img src="../hostedContents/{tempID}/$value">. An optional caption is
// rendered above the image. contentType is the image MIME type (e.g.
// "image/png"); it defaults to image/png when empty.
//
// This still posts JSON to the same endpoint as SendMessage, so it funnels
// through do() unchanged.
func (c *Client) SendImageMessage(ctx context.Context, chatID string, img []byte, contentType, caption string) (*Message, error) {
	if len(img) == 0 {
		return nil, fmt.Errorf("SendImageMessage: empty image")
	}
	if contentType == "" {
		contentType = "image/png"
	}

	const tempID = "1"
	var content bytes.Buffer
	content.WriteString("<div>")
	if caption != "" {
		// Escape so a caption that happens to contain HTML is shown literally.
		content.WriteString("<p>" + html.EscapeString(caption) + "</p>")
	}
	fmt.Fprintf(&content, `<img src="../hostedContents/%s/$value" alt="image">`, tempID)
	content.WriteString("</div>")

	payload := map[string]any{
		"body": map[string]string{
			"contentType": "html",
			"content":     content.String(),
		},
		"hostedContents": []map[string]any{
			{
				"@microsoft.graph.temporaryId": tempID,
				"contentBytes":                 base64.StdEncoding.EncodeToString(img),
				"contentType":                  contentType,
			},
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/me/chats/%s/messages", url.PathEscape(chatID))
	var msg Message
	if err := c.do(ctx, http.MethodPost, path, bytes.NewReader(buf), nil, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// FetchHostedContent downloads the raw bytes of an inline/hosted image so it can
// be written to a temp file and opened externally. urlOrPath may be an absolute
// Graph URL (as found in an <img src=...> hostedContents reference) or a
// relative path; either way it is fetched with the bearer token. Unlike do(),
// which decodes JSON, this returns the unparsed body plus the response
// Content-Type so the caller can pick a sensible file extension.
func (c *Client) FetchHostedContent(ctx context.Context, urlOrPath string) ([]byte, string, error) {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, "", err
	}

	endpoint := urlOrPath
	if len(urlOrPath) == 0 || urlOrPath[0] == '/' {
		endpoint = c.baseURL + urlOrPath
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", parseAPIError(resp, http.MethodGet, urlOrPath)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", endpoint, err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// UpcomingEvents returns calendar events occurring within the lookahead window
// from now, used to fire meeting-start notifications. Times are requested in
// UTC for consistent parsing.
func (c *Client) UpcomingEvents(ctx context.Context, lookahead time.Duration) ([]Event, error) {
	now := time.Now().UTC()
	end := now.Add(lookahead)

	q := url.Values{}
	q.Set("startDateTime", now.Format(time.RFC3339))
	q.Set("endDateTime", end.Format(time.RFC3339))
	q.Set("$orderby", "start/dateTime")
	q.Set("$top", "25")

	path := "/me/calendarView?" + q.Encode()
	headers := map[string]string{
		"Prefer": `outlook.timezone="UTC"`,
	}
	var resp listResponse[Event]
	if err := c.do(ctx, http.MethodGet, path, nil, headers, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}
