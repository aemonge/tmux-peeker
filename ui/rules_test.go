package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func TestLabeledRuleFillsExactWidth(t *testing.T) {
	for width := 12; width <= 120; width += 7 {
		got := labeledRule(ruleHeroFill, "2:logs", width)
		if w := ansi.StringWidth(got); w != width {
			t.Fatalf("width %d: rendered width = %d\n%s", width, w, got)
		}
	}
}

func TestLabeledRulePlacesGlyphsInOrder(t *testing.T) {
	got := ansi.Strip(labeledRule(ruleQueueFill, "2:logs", 60))
	if !strings.HasPrefix(got, ruleCapLeft+" ") {
		t.Fatalf("rule must open with %q: %q", ruleCapLeft, got)
	}
	if !strings.HasSuffix(got, " "+ruleCapRight) {
		t.Fatalf("rule must close with %q: %q", ruleCapRight, got)
	}
	if !strings.Contains(got, ruleGuardLeft+" 2:logs "+ruleGuardRight) {
		t.Fatalf("rule must carry guarded label: %q", got)
	}
	trimmed := strings.ReplaceAll(got, ruleQueueFill, "")
	if strings.Count(got, ruleQueueFill) < 2*ruleMinSideFill {
		t.Fatalf("rule fill too short beside label: %q", got)
	}
	if trimmed == "" {
		t.Fatal("rule has no fill")
	}
}

func TestLabeledRuleTruncatesLongLabels(t *testing.T) {
	long := strings.Repeat("window-", 20)
	for width := 16; width <= 80; width += 8 {
		got := labeledRule(ruleHeroFill, long, width)
		if w := ansi.StringWidth(got); w != width {
			t.Fatalf("width %d: rendered width = %d", width, w)
		}
		plain := ansi.Strip(got)
		if strings.Contains(plain, "window-window") && !strings.Contains(plain, "…") {
			t.Fatalf("width %d: untruncated label kept: %q", width, plain)
		}
	}
}

func TestRulesUseThemeRoles(t *testing.T) {
	useSolarizedTrueColor(t)
	green := rgb{r: 121, g: 116, b: 14} // #79740E green-brown

	assertRuleColors(t, queueRule(40, "1:server"), green)
	assertRuleColors(t, heroHeaderRule(40, "2:logs"), green)
}

// assertRuleColors verifies that every visible cell of a rule carries one
// of the two expected colors and that both colors actually appear.
func assertRuleColors(t *testing.T, rendered string, want ...rgb) {
	t.Helper()
	seen := make(map[rgb]bool)
	var foreground *rgb
	cells := 0
	for offset := 0; offset < len(rendered); {
		if rendered[offset] == '\x1b' && offset+1 < len(rendered) && rendered[offset+1] == '[' {
			end := strings.IndexByte(rendered[offset+2:], 'm')
			if end >= 0 {
				applyForegroundSGR(rendered[offset+2:offset+2+end], &foreground)
				offset += end + 3
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(rendered[offset:])
		if r != '\n' {
			cells++
			if foreground == nil {
				t.Fatalf("cell %q at %d has no foreground color", r, offset)
			}
			matched := false
			for _, color := range want {
				if approximatelyEqualRGB(*foreground, color) {
					matched = true
					seen[color] = true
				}
			}
			if !matched {
				t.Fatalf("cell %q color %v not in %v", r, *foreground, want)
			}
		}
		offset += size
	}
	if cells == 0 {
		t.Fatal("rule rendered no visible cells")
	}
	for _, color := range want {
		if !seen[color] {
			t.Fatalf("expected color %v never used", color)
		}
	}
}
