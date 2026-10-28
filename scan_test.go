package kulynflags

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// keysOf returns the sorted keys found in source, so that results can be compared directly.
func keysOf(source string, functions ...string) []string {
	var keys []string
	for key := range FindFlagKeys(source, functions...) {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestFindFlagKeysCalls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{name: "IsEnabled", source: `IsEnabled("checkout.new-ui")`, want: []string{"checkout.new-ui"}},
		{name: "GetFlag", source: `GetFlag("a")`, want: []string{"a"}},
		{name: "Flag", source: `Flag("a-b")`, want: []string{"a-b"}},
		{name: "Evaluate", source: `Evaluate("a1.b2")`, want: []string{"a1.b2"}},
		{name: "method call", source: `client.IsEnabled("via.member")`, want: []string{"via.member"}},
		{name: "extra arguments", source: `IsEnabled("with.options", opts)`, want: []string{"with.options"}},
		{name: "inside if", source: `if IsEnabled("in.if") { }`, want: []string{"in.if"}},
		{name: "multi-line with trailing comma", source: "GetFlag(\n\t\"multi.line\",\n)", want: []string{"multi.line"}},
		{name: "spaces around parentheses", source: `IsEnabled ( "spaced" )`, want: []string{"spaced"}},
		{name: "comment between tokens", source: `IsEnabled(/* note */ "x")`, want: []string{"x"}},
		{name: "two calls on one line", source: `IsEnabled("a") && GetFlag("b")`, want: []string{"a", "b"}},
		{name: "duplicate keys collapse", source: `IsEnabled("dup"); GetFlag("dup")`, want: []string{"dup"}},
		{name: "nested call", source: `IsEnabled(GetFlag("inner"))`, want: []string{"inner"}},
		{name: "invalid-looking literal is returned", source: `IsEnabled("Not Valid")`, want: []string{"Not Valid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keysOf(tt.source); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindFlagKeysQuoteVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{name: "double quotes match", source: `IsEnabled("ok")`, want: []string{"ok"}},
		{name: "backticks do not match", source: "IsEnabled(`checkout.new-ui`)", want: nil},
		{name: "template literal with placeholder does not match", source: "GetFlag(`x${suffix}`)", want: nil},
		{name: "single quotes are not Go strings", source: `IsEnabled('x')`, want: nil},
		{name: "escaped double quote inside literal", source: `IsEnabled("a\"b")`, want: nil},
		{name: "escaped backslash makes key invalid", source: `IsEnabled("a\\b")`, want: nil},
		{name: "single quote inside double quotes", source: `IsEnabled("it's")`, want: []string{"it's"}},
		{name: "rune literal with double quote does not open a string",
			source: "r := '\"'\nIsEnabled(\"after.rune\")", want: []string{"after.rune"}},
		{name: "rune literal with escape", source: `c := '\''; IsEnabled("after.escape")`,
			want: []string{"after.escape"}},
		{name: "unterminated literal does not hide the next line",
			source: "IsEnabled(\"open)\nIsEnabled(\"real\")", want: []string{"real"}},
		{name: "unterminated literal at end of file", source: `IsEnabled("open)`, want: nil},
		{name: "empty key", source: `IsEnabled("")`, want: nil},
		{name: "key of 100 bytes", source: `IsEnabled("` + strings.Repeat("a", 100) + `")`,
			want: []string{strings.Repeat("a", 100)}},
		{name: "key of 101 bytes is ignored", source: `IsEnabled("` + strings.Repeat("a", 101) + `")`, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keysOf(tt.source); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindFlagKeysIgnoresNonLiteralArguments(t *testing.T) {
	t.Parallel()
	sources := []string{
		`IsEnabled(key)`,
		`IsEnabled("a" + suffix)`,
		`IsEnabled(prefix + "a")`,
		`IsEnabled(fmt.Sprintf("a%s", x))`,
		`IsEnabled(keys["a"])`,
	}
	for _, source := range sources {
		if got := keysOf(source); got != nil {
			t.Errorf("%s: keys = %v, want none", source, got)
		}
	}
}

func TestFindFlagKeysRequiresWholeFunctionName(t *testing.T) {
	t.Parallel()
	sources := []string{
		`myIsEnabled("x")`,
		`IsEnabledAsync("x")`,
		`GetFlagValue("x")`,
		`GetFlag2("x")`,
		`isEnabled("lowercase")`,
	}
	for _, source := range sources {
		if got := keysOf(source); got != nil {
			t.Errorf("%s: keys = %v, want none", source, got)
		}
	}
}

func TestFindFlagKeysIgnoresCommentsAndStringContents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{name: "line comment", source: "// IsEnabled(\"commented\")\nIsEnabled(\"real\")", want: []string{"real"}},
		{name: "block comment", source: `/* GetFlag("commented") */ GetFlag("real")`, want: []string{"real"}},
		{name: "multi-line block comment", source: "/*\n IsEnabled(\"x\")\n*/", want: nil},
		{name: "comment marker inside string", source: `u := "https://example.com/*"; IsEnabled("real")`,
			want: []string{"real"}},
		{name: "call text inside double-quoted string", source: `s := "IsEnabled(\"fake\")"`, want: nil},
		{name: "call text inside raw string", source: "s := `IsEnabled(\"fake\")`", want: nil},
		{name: "raw string spanning lines", source: "s := `\nIsEnabled(\"fake\")\n`\nIsEnabled(\"real\")",
			want: []string{"real"}},
		{name: "backtick inside double-quoted string does not start raw string",
			source: "q := \"`\"; IsEnabled(\"after.quote\")", want: []string{"after.quote"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keysOf(tt.source); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindFlagKeysFunctionList(t *testing.T) {
	t.Parallel()
	source := `IsEnabled("a"); HasFeature("b"); IsOn("c")`
	tests := []struct {
		name      string
		functions []string
		want      []string
	}{
		{name: "defaults when no names are given", functions: nil, want: []string{"a"}},
		{name: "custom names replace the defaults", functions: []string{"HasFeature", "IsOn"}, want: []string{"b", "c"}},
		{name: "names are matched exactly", functions: []string{"HasFeat"}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keysOf(source, tt.functions...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindFlagKeysEmptyInput(t *testing.T) {
	t.Parallel()
	if got := FindFlagKeys(""); got == nil || len(got) != 0 {
		t.Fatalf("FindFlagKeys(\"\") = %#v, want empty non-nil map", got)
	}
}

// TestFindFlagKeysLongInputIsFast checks that adversarial inputs are scanned in linear time.
// Each case is at least a megabyte, and must finish well under one second.
func TestFindFlagKeysLongInputIsFast(t *testing.T) {
	t.Parallel()
	const size = 1 << 20
	tests := []struct {
		name   string
		source string
	}{
		{name: "many calls", source: strings.Repeat(`IsEnabled("a",`, size/14)},
		{name: "unclosed calls", source: strings.Repeat("IsEnabled(", size/10)},
		{name: "unterminated string", source: `IsEnabled("` + strings.Repeat("a", size)},
		{name: "many quotes", source: strings.Repeat(`"`, size)},
		{name: "escapes", source: `IsEnabled("` + strings.Repeat(`\`, size)},
		{name: "unclosed block comments", source: strings.Repeat("/*", size/2)},
		{name: "unclosed raw string", source: "`" + strings.Repeat("IsEnabled(\"x\")\n", size/16)},
		{name: "long identifier", source: strings.Repeat("a", size)},
		{name: "long argument list", source: "IsEnabled(" + strings.Repeat(`"a" + `, size/6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			_ = FindFlagKeys(tt.source)
			if elapsed := time.Since(start); elapsed >= time.Second {
				t.Fatalf("scan took %v, want < 1s", elapsed)
			}
		})
	}
}
