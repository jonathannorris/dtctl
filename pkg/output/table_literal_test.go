package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// A literal column holds identifiers that may read like a status, such as a
// Feature Management variant named "enabled": it must not be colored as one,
// while a status column next to it still is.
func TestTablePrinter_LiteralColumnSkipsStatusColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	type row struct {
		Targeting string `table:"TARGETING"`
		Variant   string `table:"VARIANT,literal"`
		Name      string `table:"NAME,wide,literal"`
	}
	for _, wide := range []bool{false, true} {
		var buf bytes.Buffer
		p := &TablePrinter{writer: &buf, wide: wide}
		if err := p.PrintList([]row{{Targeting: "on", Variant: "disabled", Name: "enabled"}}); err != nil {
			t.Fatal(err)
		}
		got := buf.String()
		if !strings.Contains(got, BrightGreen+"on") {
			t.Errorf("wide=%v: status column not colored:\n%q", wide, got)
		}
		if strings.Contains(got, BrightRed) || strings.Contains(got, BrightGreen+"enabled") {
			t.Errorf("wide=%v: literal column colored:\n%q", wide, got)
		}
		if wide != strings.Contains(got, "NAME") {
			t.Errorf("wide=%v: wide-only literal column visibility wrong:\n%s", wide, got)
		}
	}
}

func TestTrueColorEnabled(t *testing.T) {
	defer ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	for _, tc := range []struct {
		colorterm string
		want      bool
	}{{"truecolor", true}, {"24bit", true}, {"", false}, {"256color", false}} {
		ResetColorCache()
		t.Setenv("COLORTERM", tc.colorterm)
		if got := TrueColorEnabled(); got != tc.want {
			t.Errorf("COLORTERM=%q: TrueColorEnabled() = %v", tc.colorterm, got)
		}
	}
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLORTERM", "truecolor")
	if TrueColorEnabled() {
		t.Error("NO_COLOR must turn 24-bit color off too")
	}
	if got := RGB(46, 91, 214); got != "\033[38;2;46;91;214m" {
		t.Errorf("RGB = %q", got)
	}
}
