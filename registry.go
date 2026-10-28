package kulynflags

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// keyPattern is the format every flag key must match.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9-.]{1,79}$`)

// day is the unit of WithMaxLifetime. A day is exactly 24 hours.
const day = 24 * time.Hour

// maxLifetimeLimitDays bounds WithMaxLifetime (100 years), so that day counts
// cannot overflow time.Duration.
const maxLifetimeLimitDays = 36500

// Registry is a validated flag register. It is immutable after NewRegistry returns.
type Registry struct {
	flags []FlagDefinition
	// maxLifetimeDays maps each kind to its maximum lifetime in days. 0 means no limit.
	maxLifetimeDays map[Kind]int
}

// Option configures a Registry in NewRegistry.
type Option func(*registryConfig) error

type registryConfig struct {
	maxLifetimeDays map[Kind]int
}

// WithMaxLifetime sets the maximum lifetime in days for flags of the given kind.
// Zero removes the limit. A kind with a limit requires an expiry date on every flag.
//
// Defaults: release 90 days, experiment 30 days, ops 365 days, permission no limit.
func WithMaxLifetime(kind Kind, days int) Option {
	return func(cfg *registryConfig) error {
		if !kind.valid() {
			return &RegistryError{
				Code:    ErrInvalidKind,
				Message: fmt.Sprintf("WithMaxLifetime: unknown flag kind %q", kind),
			}
		}
		if days < 0 || days > maxLifetimeLimitDays {
			return &RegistryError{
				Code: ErrInvalidOption,
				Message: fmt.Sprintf(
					"WithMaxLifetime: days for kind %q must be between 0 and %d, got %d",
					kind, maxLifetimeLimitDays, days,
				),
			}
		}
		cfg.maxLifetimeDays[kind] = days
		return nil
	}
}

// NewRegistry validates the flag definitions and returns a Registry.
//
// It returns a *RegistryError for the first invalid entry. Use errors.Is with the
// Err* codes, or errors.As to read Key. Flags whose lifetime exceeds the maximum
// for their kind are accepted here. CheckHygiene reports them, so that a limit
// can be lowered without breaking existing registers at construction time.
func NewRegistry(defs []FlagDefinition, opts ...Option) (*Registry, error) {
	cfg := registryConfig{maxLifetimeDays: defaultMaxLifetimeDays()}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	seen := make(map[string]struct{}, len(defs))
	flags := make([]FlagDefinition, 0, len(defs))
	for _, def := range defs {
		if !keyPattern.MatchString(def.Key) {
			return nil, &RegistryError{
				Code: ErrInvalidKey,
				Key:  def.Key,
				Message: fmt.Sprintf(
					"invalid flag key %q: must match %s", def.Key, keyPattern.String(),
				),
			}
		}
		if _, dup := seen[def.Key]; dup {
			return nil, &RegistryError{
				Code:    ErrDuplicateKey,
				Key:     def.Key,
				Message: fmt.Sprintf("duplicate flag key %q", def.Key),
			}
		}
		seen[def.Key] = struct{}{}

		if err := validateFields(def, cfg.maxLifetimeDays); err != nil {
			return nil, err
		}
		flags = append(flags, cloneDefinition(def))
	}

	return &Registry{flags: flags, maxLifetimeDays: cfg.maxLifetimeDays}, nil
}

// defaultMaxLifetimeDays returns the default limit for each kind. 0 means no limit.
func defaultMaxLifetimeDays() map[Kind]int {
	return map[Kind]int{
		KindRelease:    90,
		KindExperiment: 30,
		KindOps:        365,
		KindPermission: 0,
	}
}

// validateFields checks everything except the key, which NewRegistry checks itself.
func validateFields(def FlagDefinition, limits map[Kind]int) error {
	if strings.TrimSpace(def.Owner) == "" {
		return &RegistryError{
			Code:    ErrMissingOwner,
			Key:     def.Key,
			Message: fmt.Sprintf("flag %q has no owner", def.Key),
		}
	}
	if !def.Kind.valid() {
		return &RegistryError{
			Code:    ErrInvalidKind,
			Key:     def.Key,
			Message: fmt.Sprintf("flag %q has unknown kind %q", def.Key, def.Kind),
		}
	}
	if def.CreatedAt.IsZero() {
		return &RegistryError{
			Code:    ErrInvalidDates,
			Key:     def.Key,
			Message: fmt.Sprintf("flag %q has no createdAt", def.Key),
		}
	}
	if def.ExpiresAt != nil && !def.ExpiresAt.After(def.CreatedAt) {
		return &RegistryError{
			Code: ErrInvalidDates,
			Key:  def.Key,
			Message: fmt.Sprintf(
				"flag %q expires at %s, which is not after createdAt %s",
				def.Key, def.ExpiresAt.Format(time.RFC3339), def.CreatedAt.Format(time.RFC3339),
			),
		}
	}
	if limits[def.Kind] > 0 && def.ExpiresAt == nil {
		return &RegistryError{
			Code: ErrMissingExpiry,
			Key:  def.Key,
			Message: fmt.Sprintf(
				"flag %q (%s) needs an expiresAt, the maximum lifetime is %d days",
				def.Key, def.Kind, limits[def.Kind],
			),
		}
	}
	return nil
}

// cloneDefinition copies def, including the ExpiresAt pointer, so that later changes
// by the caller do not affect the registry.
func cloneDefinition(def FlagDefinition) FlagDefinition {
	if def.ExpiresAt != nil {
		expires := *def.ExpiresAt
		def.ExpiresAt = &expires
	}
	return def
}
