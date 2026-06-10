package graph

import (
	"html"
	"regexp"
	"strings"
)

// ImageRef is a single image referenced by a message: an inline <img> embedded
// in the body HTML or an image attachment. Name is a human label (a filename
// when we can derive one, otherwise "image") used for the placeholder shown in
// the UI; URL is the source to fetch/open (often a Graph hostedContents
// "$value" URL that must be downloaded with auth via FetchHostedContent).
type ImageRef struct {
	Name string
	URL  string
}

var (
	// imgTagRe matches an <img ...> tag so we can pull its attributes. (?is)
	// makes it case-insensitive and dot-matches-newline, since Teams body HTML
	// is a single string with arbitrary casing and embedded newlines.
	imgTagRe = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	// imgSrcRe extracts the src value, tolerant of single or double quotes and
	// surrounding attributes in any order.
	imgSrcRe = regexp.MustCompile(`(?is)\bsrc\s*=\s*("([^"]*)"|'([^']*)')`)
	// imgAltRe / imgItemIDRe derive a friendly label when present; alt is
	// preferred (it often carries the original filename), itemid is a stable
	// fallback identifier Teams puts on inline images.
	imgAltRe    = regexp.MustCompile(`(?is)\balt\s*=\s*("([^"]*)"|'([^']*)')`)
	imgItemIDRe = regexp.MustCompile(`(?is)\bitemid\s*=\s*("([^"]*)"|'([^']*)')`)
)

// attrValue returns the captured quoted value from a submatch produced by the
// regexps above, which place the double-quoted body in group 2 and the
// single-quoted body in group 3.
func attrValue(m []string) string {
	if len(m) >= 4 {
		if m[2] != "" {
			return m[2]
		}
		return m[3]
	}
	return ""
}

// Images returns every image referenced by the message: inline <img> tags in
// the body HTML plus any attachments with an "image/" content type. We scan the
// raw HTML (not the PlainText output, which strips tags) so the UI can render a
// "[image: name]" placeholder per image and later download it. Results are
// de-duplicated by URL and nil is returned when the message has no images.
func (m *Message) Images() []ImageRef {
	var refs []ImageRef
	seen := make(map[string]bool)

	add := func(name, u string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		if name == "" {
			name = "image"
		}
		refs = append(refs, ImageRef{Name: name, URL: u})
	}

	// Inline images embedded in the (HTML) body.
	if strings.EqualFold(m.Body.ContentType, "html") {
		for _, tag := range imgTagRe.FindAllString(m.Body.Content, -1) {
			src := attrValue(imgSrcRe.FindStringSubmatch(tag))
			if src == "" {
				continue
			}
			// Entities like &amp; appear in HTML attribute values; decode so
			// the URL is usable.
			src = html.UnescapeString(src)
			name := html.UnescapeString(attrValue(imgAltRe.FindStringSubmatch(tag)))
			if name == "" {
				name = html.UnescapeString(attrValue(imgItemIDRe.FindStringSubmatch(tag)))
			}
			add(name, src)
		}
	}

	// Image attachments carried alongside the body.
	for _, a := range m.Attachments {
		if strings.HasPrefix(strings.ToLower(a.ContentType), "image/") {
			add(a.Name, a.ContentURL)
		}
	}

	return refs
}

// HasImages reports whether the message references any image, letting callers
// cheaply decide whether to render image placeholders.
func (m *Message) HasImages() bool {
	return len(m.Images()) > 0
}
