// Package all registers every compiled-in wizard.
//
// As with checks and fixes, this import list is the whole set. A wizard that is
// not named here cannot be started, and the steps it would have run cannot be
// reached.
package all

import (
	_ "github.com/ZanOzair/SupportOne/internal/wizard/connection"
	_ "github.com/ZanOzair/SupportOne/internal/wizard/printing"
)
