package kulynflags

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// mustRegistry builds a registry from defs and fails the test on error.
func mustRegistry(t *testing.T, defs []FlagDefinition, opts ...Option) *Registry {
	t.Helper()
	reg, err := NewRegistry(defs, opts...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// findingPairs turns findings into "key:CODE" strings, so that tests can compare them compactly.
func findingPairs(findings []Finding) []string {
	var pairs []string
	for _, f := range findings {
		pairs = append(pairs, f.Key+":"+string(f.Code))
	}
	return pairs
}

func TestCheckHygieneExpiry(t *testing.T) {
	t.Parallel()
	expires := testCreated.Add(10 * day)
	tests := []struct {
		name string
		now  time.Time
		want []string
	}{
		{name: "one nanosecond before expiry", now: expires.Add(-time.Nanosecond), want: nil},
		{name: "exactly at expiry", now: expires, want: []string{"a.flag:EXPIRED"}},
		{name: "after expiry", now: expires.Add(time.Second), want: []string{"a.flag:EXPIRED"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := mustRegistry(t, []FlagDefinition{flagWith("a.flag", KindRelease)})
			got := findingPairs(reg.CheckHygiene(tt.now, nil))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckHygienePermissionWithoutExpiryNeverExpires(t *testing.T) {
	t.Parallel()
	def := FlagDefinition{Key: "a.perm", Owner: "team", Kind: KindPermission, CreatedAt: testCreated}
	reg := mustRegistry(t, []FlagDefinition{def})
	far := testCreated.Add(100 * 365 * day)
	if got := reg.CheckHygiene(far, nil); len(got) != 0 {
		t.Fatalf("permission without expiry produced findings: %v", findingPairs(got))
	}
}

func TestCheckHygieneLifetimeBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		kind     Kind
		lifetime time.Duration
		opts     []Option
		want     []string
	}{
		{name: "release exactly 90 days", kind: KindRelease, lifetime: 90 * day, want: nil},
		{name: "release 90 days plus one second", kind: KindRelease, lifetime: 90*day + time.Second,
			want: []string{"a.flag:EXCEEDS_MAX_LIFETIME"}},
		{name: "experiment exactly 30 days", kind: KindExperiment, lifetime: 30 * day, want: nil},
		{name: "experiment 31 days", kind: KindExperiment, lifetime: 31 * day,
			want: []string{"a.flag:EXCEEDS_MAX_LIFETIME"}},
		{name: "ops exactly 365 days", kind: KindOps, lifetime: 365 * day, want: nil},
		{name: "ops 366 days", kind: KindOps, lifetime: 366 * day,
			want: []string{"a.flag:EXCEEDS_MAX_LIFETIME"}},
		{name: "permission has no limit", kind: KindPermission, lifetime: 5000 * day, want: nil},
		{name: "release limit removed by zero", kind: KindRelease, lifetime: 1000 * day,
			opts: []Option{WithMaxLifetime(KindRelease, 0)}, want: nil},
		{name: "release limit lowered to 5 days", kind: KindRelease, lifetime: 6 * day,
			opts: []Option{WithMaxLifetime(KindRelease, 5)}, want: []string{"a.flag:EXCEEDS_MAX_LIFETIME"}},
		{name: "release at lowered limit of 5 days", kind: KindRelease, lifetime: 5 * day,
			opts: []Option{WithMaxLifetime(KindRelease, 5)}, want: nil},
		{name: "permission gets a limit", kind: KindPermission, lifetime: 31 * day,
			opts: []Option{WithMaxLifetime(KindPermission, 30)}, want: []string{"a.flag:EXCEEDS_MAX_LIFETIME"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := FlagDefinition{
				Key:       "a.flag",
				Owner:     "team",
				Kind:      tt.kind,
				CreatedAt: testCreated,
				ExpiresAt: after(tt.lifetime),
			}
			reg := mustRegistry(t, []FlagDefinition{def}, tt.opts...)
			got := findingPairs(reg.CheckHygiene(testCreated, nil))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckHygieneUnusedOnlyWithUsedList(t *testing.T) {
	t.Parallel()
	reg := mustRegistry(t, []FlagDefinition{
		flagWith("a.one", KindRelease),
		flagWith("a.two", KindRelease),
	})
	tests := []struct {
		name string
		used []string
		want []string
	}{
		{name: "nil used list reports no usage findings", used: nil, want: nil},
		{name: "empty used list reports every flag as unused", used: []string{},
			want: []string{"a.one:UNUSED", "a.two:UNUSED"}},
		{name: "all flags used", used: []string{"a.one", "a.two"}, want: nil},
		{name: "one flag used", used: []string{"a.two"}, want: []string{"a.one:UNUSED"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := findingPairs(reg.CheckHygiene(testCreated, tt.used))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckHygieneUnregisteredUsage(t *testing.T) {
	t.Parallel()
	reg := mustRegistry(t, []FlagDefinition{flagWith("a.known", KindRelease)})
	got := reg.CheckHygiene(testCreated, []string{"a.known", "a.ghost", "a.ghost", "b.ghost"})
	want := []string{"a.ghost:UNREGISTERED_USAGE", "b.ghost:UNREGISTERED_USAGE"}
	if pairs := findingPairs(got); !reflect.DeepEqual(pairs, want) {
		t.Fatalf("findings = %v, want %v", pairs, want)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, f.Key) {
			t.Errorf("message %q does not name key %q", f.Message, f.Key)
		}
	}
}

func TestCheckHygieneSortOrder(t *testing.T) {
	t.Parallel()
	// Registered in reverse key order to show that the result is sorted, not in input order.
	reg := mustRegistry(t, []FlagDefinition{
		flagWith("z.flag", KindRelease),
		{
			Key:       "a.flag",
			Owner:     "team",
			Kind:      KindOps,
			CreatedAt: testCreated,
			ExpiresAt: after(400 * day), // above the ops limit of 365 days
		},
	})
	now := testCreated.Add(11 * day) // z.flag has expired
	got := findingPairs(reg.CheckHygiene(now, []string{"m.ghost"}))
	want := []string{
		"a.flag:EXCEEDS_MAX_LIFETIME",
		"a.flag:UNUSED",
		"m.ghost:UNREGISTERED_USAGE",
		"z.flag:EXPIRED",
		"z.flag:UNUSED",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}
}

func TestCheckHygieneIsDeterministic(t *testing.T) {
	t.Parallel()
	reg := mustRegistry(t, []FlagDefinition{
		flagWith("b.one", KindRelease),
		flagWith("a.two", KindExperiment),
		flagWith("c.three", KindOps),
	})
	used := []string{"x.1", "y.2", "z.3", "a.two"}
	first := reg.CheckHygiene(testCreated.Add(20*day), used)
	for range 20 {
		if got := reg.CheckHygiene(testCreated.Add(20*day), used); !reflect.DeepEqual(got, first) {
			t.Fatalf("result changed between calls:\nfirst: %v\ngot:   %v", first, got)
		}
	}
}

func TestCheckHygieneEmptyRegisterReturnsNonNil(t *testing.T) {
	t.Parallel()
	reg := mustRegistry(t, nil)
	got := reg.CheckHygiene(testCreated, nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("CheckHygiene on empty register = %#v, want empty non-nil slice", got)
	}
}

func TestAssertHygiene(t *testing.T) {
	t.Parallel()
	expired := Finding{Key: "a.one", Code: FindingExpired, Message: "expired"}
	unused := Finding{Key: "a.two", Code: FindingUnused, Message: "unused"}
	tests := []struct {
		name     string
		findings []Finding
		failOn   []FindingCode
		wantKeys []string
	}{
		{name: "no findings", findings: nil, failOn: nil, wantKeys: nil},
		{name: "no findings with failOn", findings: nil, failOn: []FindingCode{FindingExpired}, wantKeys: nil},
		{name: "any finding fails without failOn", findings: []Finding{unused}, wantKeys: []string{"a.two"}},
		{name: "all findings are listed without failOn", findings: []Finding{expired, unused},
			wantKeys: []string{"a.one", "a.two"}},
		{name: "failOn code not present passes", findings: []Finding{unused},
			failOn: []FindingCode{FindingExpired}, wantKeys: nil},
		{name: "failOn code present fails with matching findings only", findings: []Finding{expired, unused},
			failOn: []FindingCode{FindingExpired}, wantKeys: []string{"a.one"}},
		{name: "failOn several codes", findings: []Finding{expired, unused},
			failOn: []FindingCode{FindingUnused, FindingExpired}, wantKeys: []string{"a.one", "a.two"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := AssertHygiene(tt.findings, tt.failOn...)
			if tt.wantKeys == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			var hygieneErr *HygieneError
			if !errors.As(err, &hygieneErr) {
				t.Fatalf("expected *HygieneError, got %v", err)
			}
			var keys []string
			for _, f := range hygieneErr.Findings {
				keys = append(keys, f.Key)
			}
			if !reflect.DeepEqual(keys, tt.wantKeys) {
				t.Fatalf("error findings = %v, want %v", keys, tt.wantKeys)
			}
			if !strings.Contains(err.Error(), "finding(s)") {
				t.Errorf("error message does not summarize findings: %q", err.Error())
			}
		})
	}
}
