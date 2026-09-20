package static

import "embed"

// FS embeds the compiled frontend assets (CSS, JS) so the production binary
// is self-contained.
//
//go:embed all:css all:js
var FS embed.FS
