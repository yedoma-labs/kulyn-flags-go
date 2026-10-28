# kulyn-flags-go
<picture>
  <source media="(max-width: 640px)" srcset="https://raw.githubusercontent.com/yedoma-labs/assets/main/resized/banner-resized-mobile.png">
  <img src="https://raw.githubusercontent.com/yedoma-labs/assets/main/resized/banner-resized.png" alt="Project Header">
</picture>

A feature flag register for Go with owners, expiry dates and CI hygiene checks.

Go module: `github.com/yedoma-labs/kulyn-flags-go`. Go 1.23 or newer. No dependencies outside the standard library.

## Problem

Feature flags are easy to add and hard to remove. Over time a codebase collects flags that:

- nobody remembers creating,
- nobody owns, so nobody feels responsible for removing them,
- are still in the code long after the rollout ended, and
- cannot be deleted safely, because nobody knows whether they are still used.

Runtime SDKs such as OpenFeature, Unleash and Flagsmith evaluate flags well. They do not track who owns a flag, when it should be removed, or whether it is still referenced in the code. In Go there was no library for this gap.

kulyn-flags-go keeps that information in a register that lives next to your code. A test in CI checks the register against the code and fails when a flag is expired, too old, unused, or used without being registered.

## Quickstart

### 1. Register your flags

```go
package flags

import (
	"time"

	kulynflags "github.com/yedoma-labs/kulyn-flags-go"
)

func Register() ([]kulynflags.FlagDefinition, error) {
	created := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	expires := created.AddDate(0, 0, 60)

	return []kulynflags.FlagDefinition{
		{
			Key:         "checkout.new-ui",
			Owner:       "team-checkout",
			Kind:        kulynflags.KindRelease,
			CreatedAt:   created,
			ExpiresAt:   &expires,
			Description: "New checkout layout",
		},
	}, nil
}
```

`NewRegistry` validates the register once, at startup or in a test:

```go
defs, _ := flags.Register()
reg, err := kulynflags.NewRegistry(defs)
if err != nil {
	// errors.Is(err, kulynflags.ErrInvalidKey), ErrMissingOwner, ... and errors.As for details
}
```

### 2. Add a hygiene test to CI

Scan your source for flag keys with `FindFlagKeys`, then check the register against them:

```go
package flags_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kulynflags "github.com/yedoma-labs/kulyn-flags-go"
)

func usedFlagKeys(t *testing.T) []string {
	t.Helper()
	found := map[string]struct{}{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for key := range kulynflags.FindFlagKeys(string(src)) {
			found[key] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	return keys
}

func TestFlagHygiene(t *testing.T) {
	defs, err := flags.Register()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := kulynflags.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	findings := reg.CheckHygiene(time.Now(), usedFlagKeys(t))
	if err := kulynflags.AssertHygiene(findings); err != nil {
		t.Fatal(err)
	}
}
```

Run it with `go test ./...`. The test fails with a message like this when a flag needs attention:

```
flag hygiene check failed with 2 finding(s):
  - [EXPIRED] flag "checkout.new-ui" expired at 2026-04-30T00:00:00Z
  - [UNUSED] flag "checkout.new-ui" is registered but not used
```

To fail only on some findings, pass the codes to `AssertHygiene`, for example `kulynflags.AssertHygiene(findings, kulynflags.FindingExpired)`.

## Concepts

### Flag kinds and lifetimes

Each flag has a `Kind`. The kind determines how long the flag may live.

| Kind                | Default maximum lifetime | Expiry required |
|---------------------|--------------------------|-----------------|
| `KindRelease`       | 90 days                  | yes             |
| `KindExperiment`    | 30 days                  | yes             |
| `KindOps`           | 365 days                 | yes             |
| `KindPermission`    | no limit                 | no              |

Change the limits with options. `0` removes the limit, and a kind without a limit no longer needs an expiry:

```go
reg, err := kulynflags.NewRegistry(defs,
	kulynflags.WithMaxLifetime(kulynflags.KindRelease, 45),
	kulynflags.WithMaxLifetime(kulynflags.KindOps, 0),
)
```

### Validation

`NewRegistry` rejects an invalid register with a `*RegistryError`. Check it with `errors.Is` against these codes:

| Code               | Meaning                                                              |
|--------------------|----------------------------------------------------------------------|
| `ErrInvalidKey`    | The key does not match `^[a-z][a-z0-9-.]{1,79}$`.                    |
| `ErrDuplicateKey`  | Two flags have the same key.                                         |
| `ErrMissingOwner`  | The owner is empty or blank.                                         |
| `ErrInvalidDates`  | `CreatedAt` is missing, or `ExpiresAt` is not after `CreatedAt`.     |
| `ErrMissingExpiry` | A kind with a maximum lifetime has no `ExpiresAt`.                   |
| `ErrInvalidKind`   | The kind is unknown.                                                 |
| `ErrInvalidOption` | An option has an invalid value, such as a negative number of days.   |

A flag that lives longer than the maximum for its kind is accepted by `NewRegistry`. `CheckHygiene` reports it as `EXCEEDS_MAX_LIFETIME`. This way a lowered limit shows up in CI, and does not break the register at startup.

### Hygiene findings

`CheckHygiene(now, used)` returns findings sorted by key, then by code.

| Code                      | Meaning                                                                                   |
|---------------------------|-------------------------------------------------------------------------------------------|
| `FindingExpired`          | `now` is at or after `ExpiresAt`. The exact moment of expiry counts as expired.           |
| `FindingExceedsMaxLifetime` | The lifetime is longer than the maximum for the kind. A lifetime equal to the maximum is allowed. |
| `FindingUnused`           | The flag is registered but not in `used`. Only checked when `used` is not `nil`.          |
| `FindingUnregisteredUsage`| A key in `used` is not in the register. Only checked when `used` is not `nil`.            |

Pass `nil` for `used` to skip the usage checks. Pass an empty, non-nil slice to report every flag as unused.

### Source scanner

`FindFlagKeys(source string, functions ...string)` returns the keys passed as double-quoted string literals to `IsEnabled`, `GetFlag`, `Flag` or `Evaluate`, or to the function names you pass. Comments, raw strings in backticks and text inside other strings are not read as code.

Limitations:

- Only literal keys are found. `IsEnabled(key)`, `IsEnabled("a" + suffix)` and keys built with `fmt.Sprintf` are not detected. Keep flag keys as literals.
- Only the function name is matched. A helper with the same name in another package is also counted. Pass narrower function names if this causes trouble.
- The function does not read files. You decide which files to scan, for example all `*.go` files except tests.

## Not a runtime SDK

kulyn-flags-go does not evaluate flags. It has no network access, no providers, no caching and no targeting rules. Use your existing SDK, or a plain config value, to decide whether a flag is on.

This library covers the part that runtime SDKs leave out: the lifecycle of a flag. It records who owns a flag, when it should be removed, and whether the code still uses it. You can use it with any flag provider, or with flags that are only constants in your code.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

## License

MIT. See [LICENSE](LICENSE).
