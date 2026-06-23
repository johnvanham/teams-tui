package graph

// The full emoji table in emoji_table.go is generated from the canonical
// github/gemoji dataset (db/emoji.json), which maps Unicode emoji to their
// GitHub/Slack-style shortcode aliases (e.g. "nauseated_face").
//
// To regenerate after a gemoji update:
//
//	curl -fsSL https://raw.githubusercontent.com/github/gemoji/master/db/emoji.json -o /tmp/gemoji.json
//	# then run a small emitter that writes []gemojiEntry from the JSON
//
// The committed emoji_table.go is the source of truth at build time; this file
// only documents provenance. We intentionally vendor the generated Go rather
// than the JSON so the package has no data-file or codegen dependency at build.
