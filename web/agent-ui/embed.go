// Package agentui embeds the built interface into the agent binary, so a user
// runs one file and nothing has to be installed alongside it.
package agentui

import "embed"

// Assets holds the built interface. Rebuild it with `npm run build` in
// web/agent-ui; the built output is committed so `go build` works without Node.
//
//go:embed all:dist
var Assets embed.FS
