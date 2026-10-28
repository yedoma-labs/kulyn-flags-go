// Package kulynflags keeps a register of feature flags with owners and expiry
// dates, and checks the register for flags that should be removed.
//
// It is not a runtime SDK. Flags are evaluated by your own code or by an existing
// SDK. This package only answers the question "which flags should be cleaned up?".
package kulynflags

import "time"

// Kind describes why a flag exists. Each kind has its own default maximum lifetime.
type Kind string

// Flag kinds.
const (
	// KindRelease guards an unfinished feature until it is released everywhere.
	KindRelease Kind = "release"
	// KindExperiment is an A/B test or other time-boxed measurement.
	KindExperiment Kind = "experiment"
	// KindOps is an operational switch, such as a kill switch or a rate limit toggle.
	KindOps Kind = "ops"
	// KindPermission gates access for a group of users. It may live without limit.
	KindPermission Kind = "permission"
)

// valid reports whether k is one of the known flag kinds.
func (k Kind) valid() bool {
	switch k {
	case KindRelease, KindExperiment, KindOps, KindPermission:
		return true
	default:
		return false
	}
}

// FlagDefinition is a single entry in the flag register.
type FlagDefinition struct {
	// Key is the flag name, for example "checkout.new-ui".
	// It must match ^[a-z][a-z0-9-.]{1,79}$.
	Key string
	// Owner is the team or person responsible for removing the flag. It must not be blank.
	Owner string
	// Kind describes why the flag exists.
	Kind Kind
	// CreatedAt is when the flag was introduced. It is required.
	CreatedAt time.Time
	// ExpiresAt is the point in time from which the flag counts as expired.
	// Nil means the flag has no expiry. Only kinds without a maximum lifetime allow nil.
	ExpiresAt *time.Time
	// Description explains what the flag does. It is optional.
	Description string
}

// FindingCode identifies the kind of a hygiene finding.
type FindingCode string

// Hygiene finding codes.
const (
	// FindingExpired means the reference time is at or after the flag's ExpiresAt.
	FindingExpired FindingCode = "EXPIRED"
	// FindingExceedsMaxLifetime means the flag lives longer than the maximum for its kind.
	// This happens when a limit was lowered after the flag was registered.
	FindingExceedsMaxLifetime FindingCode = "EXCEEDS_MAX_LIFETIME"
	// FindingUnused means the flag is registered but not used. Only reported when a
	// list of used keys is given.
	FindingUnused FindingCode = "UNUSED"
	// FindingUnregisteredUsage means a used key is not in the register. Only reported
	// when a list of used keys is given.
	FindingUnregisteredUsage FindingCode = "UNREGISTERED_USAGE"
)

// Finding is one problem reported by Registry.CheckHygiene.
type Finding struct {
	Key     string
	Code    FindingCode
	Message string
}
