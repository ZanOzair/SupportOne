package restore

import (
	"context"
	"fmt"
)

// linuxMaker reports that Linux has no system restore point to make.
//
// There is no mechanism every distribution has. Btrfs and ZFS snapshots,
// Timeshift and LVM all exist, but each depends on how the machine was set up,
// and a tool that guessed wrong would either fail or quietly do nothing while
// claiming otherwise. Saying there is no restore point is the honest answer,
// and the fixes themselves still carry their own rollback.
type linuxMaker struct{}

// New returns the restore point maker for this platform.
func New() Maker { return linuxMaker{} }

func (linuxMaker) Check(context.Context) Availability {
	return Availability{
		Kind:   "system restore point",
		Reason: KeyUnavailableOnPlatform,
	}
}

func (linuxMaker) Create(context.Context, string) (Point, error) {
	return Point{}, fmt.Errorf("restore: Linux has no system restore point this agent can make; each fix carries its own rollback")
}
