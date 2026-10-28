package kulynflags

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// CheckHygiene reports flags that should be cleaned up, as of the reference time now.
//
//   - FindingExpired: now is at or after the flag's ExpiresAt.
//   - FindingExceedsMaxLifetime: the lifetime (ExpiresAt minus CreatedAt) is longer than
//     the maximum for the flag's kind. A lifetime equal to the maximum is allowed.
//   - FindingUnused: only when used is not nil. The registered key is not in used.
//     An empty, non-nil used slice reports every flag as unused.
//   - FindingUnregisteredUsage: only when used is not nil. The used key is not registered.
//
// Findings are sorted by Key, then by Code. The result is never nil.
func (r *Registry) CheckHygiene(now time.Time, used []string) []Finding {
	findings := make([]Finding, 0)
	registered := make(map[string]struct{}, len(r.flags))

	for _, flag := range r.flags {
		registered[flag.Key] = struct{}{}
		if flag.ExpiresAt == nil {
			continue
		}
		if !now.Before(*flag.ExpiresAt) {
			findings = append(findings, Finding{
				Key:  flag.Key,
				Code: FindingExpired,
				Message: fmt.Sprintf(
					"flag %q expired at %s", flag.Key, flag.ExpiresAt.Format(time.RFC3339),
				),
			})
		}
		limit := r.maxLifetimeDays[flag.Kind]
		if limit > 0 {
			lifetime := flag.ExpiresAt.Sub(flag.CreatedAt)
			if lifetime > time.Duration(limit)*day {
				findings = append(findings, Finding{
					Key:  flag.Key,
					Code: FindingExceedsMaxLifetime,
					Message: fmt.Sprintf(
						"flag %q (%s) lives %.0f days, the maximum is %d",
						flag.Key, flag.Kind, lifetime.Hours()/24, limit,
					),
				})
			}
		}
	}

	if used != nil {
		usedSet := make(map[string]struct{}, len(used))
		for _, key := range used {
			usedSet[key] = struct{}{}
		}
		for _, flag := range r.flags {
			if _, ok := usedSet[flag.Key]; !ok {
				findings = append(findings, Finding{
					Key:     flag.Key,
					Code:    FindingUnused,
					Message: fmt.Sprintf("flag %q is registered but not used", flag.Key),
				})
			}
		}
		for key := range usedSet {
			if _, ok := registered[key]; !ok {
				findings = append(findings, Finding{
					Key:     key,
					Code:    FindingUnregisteredUsage,
					Message: fmt.Sprintf("flag %q is used but not registered", key),
				})
			}
		}
	}

	slices.SortFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			strings.Compare(a.Key, b.Key),
			strings.Compare(string(a.Code), string(b.Code)),
		)
	})
	return findings
}

// AssertHygiene returns a *HygieneError listing the findings whose code is in failOn.
// If failOn is empty, every finding counts. It returns nil when nothing matches.
// Use it in a CI test to fail the build on hygiene problems.
func AssertHygiene(findings []Finding, failOn ...FindingCode) error {
	var matched []Finding
	for _, finding := range findings {
		if len(failOn) == 0 || slices.Contains(failOn, finding.Code) {
			matched = append(matched, finding)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	return &HygieneError{Findings: matched}
}
