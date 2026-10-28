package kulynflags

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var testCreated = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// after returns a pointer to the instant d after testCreated.
func after(d time.Duration) *time.Time {
	t := testCreated.Add(d)
	return &t
}

// flagWith returns a valid definition of the given kind that expires after 10 days.
func flagWith(key string, kind Kind) FlagDefinition {
	return FlagDefinition{
		Key:       key,
		Owner:     "team-checkout",
		Kind:      kind,
		CreatedAt: testCreated,
		ExpiresAt: after(10 * day),
	}
}

func TestNewRegistryKeyFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		key  string
		want ErrorCode
	}{
		{name: "two characters", key: "ab"},
		{name: "dots and dashes", key: "a-b.c"},
		{name: "digits after first letter", key: "a1.b2"},
		{name: "typical key", key: "checkout.new-ui"},
		{name: "maximum length 80", key: "a" + strings.Repeat("b", 79)},
		{name: "empty", key: "", want: ErrInvalidKey},
		{name: "single character", key: "a", want: ErrInvalidKey},
		{name: "maximum length plus one", key: "a" + strings.Repeat("b", 80), want: ErrInvalidKey},
		{name: "uppercase", key: "Checkout", want: ErrInvalidKey},
		{name: "starts with digit", key: "1abc", want: ErrInvalidKey},
		{name: "starts with dash", key: "-abc", want: ErrInvalidKey},
		{name: "starts with dot", key: ".abc", want: ErrInvalidKey},
		{name: "underscore", key: "ab_c", want: ErrInvalidKey},
		{name: "space", key: "ab c", want: ErrInvalidKey},
		{name: "slash", key: "ab/c", want: ErrInvalidKey},
		{name: "trailing newline", key: "abc\n", want: ErrInvalidKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRegistry([]FlagDefinition{flagWith(tt.key, KindRelease)})
			assertRegistryCode(t, err, tt.want)
		})
	}
}

func TestNewRegistryRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		defs []FlagDefinition
		opts []Option
		want ErrorCode
	}{
		{
			name: "empty register is valid",
			defs: nil,
		},
		{
			name: "duplicate key",
			defs: []FlagDefinition{flagWith("a.b", KindRelease), flagWith("a.b", KindOps)},
			want: ErrDuplicateKey,
		},
		{
			name: "empty owner",
			defs: []FlagDefinition{withOwner(flagWith("a.b", KindRelease), "")},
			want: ErrMissingOwner,
		},
		{
			name: "blank owner",
			defs: []FlagDefinition{withOwner(flagWith("a.b", KindRelease), " \t ")},
			want: ErrMissingOwner,
		},
		{
			name: "unknown kind",
			defs: []FlagDefinition{flagWith("a.b", Kind("bogus"))},
			want: ErrInvalidKind,
		},
		{
			name: "zero createdAt",
			defs: []FlagDefinition{withCreated(flagWith("a.b", KindRelease), time.Time{})},
			want: ErrInvalidDates,
		},
		{
			name: "expiry equal to creation",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindRelease), after(0))},
			want: ErrInvalidDates,
		},
		{
			name: "expiry before creation",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindRelease), after(-time.Second))},
			want: ErrInvalidDates,
		},
		{
			name: "expiry one nanosecond after creation",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindRelease), after(time.Nanosecond))},
		},
		{
			name: "release without expiry",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindRelease), nil)},
			want: ErrMissingExpiry,
		},
		{
			name: "experiment without expiry",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindExperiment), nil)},
			want: ErrMissingExpiry,
		},
		{
			name: "ops without expiry",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindOps), nil)},
			want: ErrMissingExpiry,
		},
		{
			name: "permission without expiry is allowed",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindPermission), nil)},
		},
		{
			name: "permission with expiry is allowed",
			defs: []FlagDefinition{flagWith("a.b", KindPermission)},
		},
		{
			name: "release without expiry allowed after limit is removed",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindRelease), nil)},
			opts: []Option{WithMaxLifetime(KindRelease, 0)},
		},
		{
			name: "lifetime above the default maximum is accepted here",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindRelease), after(400*day))},
		},
		{
			name: "permission gets a limit through the option",
			defs: []FlagDefinition{withExpiry(flagWith("a.b", KindPermission), nil)},
			opts: []Option{WithMaxLifetime(KindPermission, 30)},
			want: ErrMissingExpiry,
		},
		{
			name: "option with unknown kind",
			defs: nil,
			opts: []Option{WithMaxLifetime(Kind("bogus"), 10)},
			want: ErrInvalidKind,
		},
		{
			name: "option with negative days",
			defs: nil,
			opts: []Option{WithMaxLifetime(KindOps, -1)},
			want: ErrInvalidOption,
		},
		{
			name: "option with days above the bound",
			defs: nil,
			opts: []Option{WithMaxLifetime(KindOps, maxLifetimeLimitDays+1)},
			want: ErrInvalidOption,
		},
		{
			name: "option with the largest allowed days",
			defs: nil,
			opts: []Option{WithMaxLifetime(KindOps, maxLifetimeLimitDays)},
		},
		{
			name: "nil option is ignored",
			defs: []FlagDefinition{flagWith("a.b", KindRelease)},
			opts: []Option{nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRegistry(tt.defs, tt.opts...)
			assertRegistryCode(t, err, tt.want)
		})
	}
}

func TestNewRegistryErrorDetails(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry([]FlagDefinition{
		flagWith("a.ok", KindRelease),
		flagWith("a.dup", KindRelease),
		flagWith("a.dup", KindRelease),
	})
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("errors.As(%v) did not yield *RegistryError", err)
	}
	if regErr.Code != ErrDuplicateKey {
		t.Errorf("Code = %q, want %q", regErr.Code, ErrDuplicateKey)
	}
	if regErr.Key != "a.dup" {
		t.Errorf("Key = %q, want %q", regErr.Key, "a.dup")
	}
	if regErr.Error() == "" {
		t.Error("Error() is empty")
	}
}

func TestNewRegistryReturnsFirstInvalidEntry(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry([]FlagDefinition{
		withOwner(flagWith("a.first", KindRelease), ""),
		flagWith("Bad Key", KindRelease),
	})
	assertRegistryCode(t, err, ErrMissingOwner)
}

func TestNewRegistryCopiesDefinitions(t *testing.T) {
	t.Parallel()
	expires := testCreated.Add(10 * day)
	defs := []FlagDefinition{{
		Key:       "a.copy",
		Owner:     "team",
		Kind:      KindRelease,
		CreatedAt: testCreated,
		ExpiresAt: &expires,
	}}
	reg, err := NewRegistry(defs)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Changing the caller's data after construction must not change the register.
	defs[0].Owner = ""
	*defs[0].ExpiresAt = testCreated.Add(400 * day)

	findings := reg.CheckHygiene(testCreated.Add(11*day), nil)
	if len(findings) != 1 || findings[0].Code != FindingExpired {
		t.Fatalf("expected one EXPIRED finding from the original expiry, got %+v", findings)
	}
}

func assertRegistryCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	if !errors.Is(err, want) {
		t.Fatalf("errors.Is(err, %q) = false, err = %v", want, err)
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("errors.As(err) did not yield *RegistryError: %v", err)
	}
	if regErr.Code != want {
		t.Fatalf("Code = %q, want %q", regErr.Code, want)
	}
}

func withOwner(def FlagDefinition, owner string) FlagDefinition {
	def.Owner = owner
	return def
}

func withCreated(def FlagDefinition, created time.Time) FlagDefinition {
	def.CreatedAt = created
	return def
}

func withExpiry(def FlagDefinition, expires *time.Time) FlagDefinition {
	def.ExpiresAt = expires
	return def
}
