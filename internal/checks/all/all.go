// Package all registers every compiled-in check.
//
// Importing this package is what puts checks into the default registry.
// Adding a thirteenth check means adding one import line here and one new
// package — never a change to the registry or the runner.
package all

import (
	_ "github.com/ZanOzair/supportone/internal/checks/drivers"
	_ "github.com/ZanOzair/supportone/internal/checks/events"
	_ "github.com/ZanOzair/supportone/internal/checks/network"
	_ "github.com/ZanOzair/supportone/internal/checks/security"
	_ "github.com/ZanOzair/supportone/internal/checks/startup"
	_ "github.com/ZanOzair/supportone/internal/checks/storage"
	_ "github.com/ZanOzair/supportone/internal/checks/system"
	_ "github.com/ZanOzair/supportone/internal/checks/updates"
)
