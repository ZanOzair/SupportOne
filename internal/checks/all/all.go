// Package all registers every compiled-in check.
//
// Importing this package is what puts checks into the default registry.
// Adding a check means adding one import line here and one new package —
// never a change to the registry or the runner.
package all

import (
	_ "github.com/ZanOzair/SupportOne/internal/checks/backup"
	_ "github.com/ZanOzair/SupportOne/internal/checks/drivers"
	_ "github.com/ZanOzair/SupportOne/internal/checks/events"
	_ "github.com/ZanOzair/SupportOne/internal/checks/network"
	_ "github.com/ZanOzair/SupportOne/internal/checks/performance"
	_ "github.com/ZanOzair/SupportOne/internal/checks/security"
	_ "github.com/ZanOzair/SupportOne/internal/checks/startup"
	_ "github.com/ZanOzair/SupportOne/internal/checks/storage"
	_ "github.com/ZanOzair/SupportOne/internal/checks/system"
	_ "github.com/ZanOzair/SupportOne/internal/checks/updates"
)
