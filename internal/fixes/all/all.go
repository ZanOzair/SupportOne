// Package all registers every compiled-in fix.
//
// This import list is the whitelist. A fix that is not named here cannot be
// planned, confirmed or applied, no matter what asks for it — a URL, a saved
// report, a model's suggestion. Adding a fix means adding one import line here
// and one new package, and nothing else in the codebase changes.
package all

import (
	_ "github.com/ZanOzair/SupportOne/internal/fixes/dns"
	_ "github.com/ZanOzair/SupportOne/internal/fixes/spooler"
	_ "github.com/ZanOzair/SupportOne/internal/fixes/temp"
)
