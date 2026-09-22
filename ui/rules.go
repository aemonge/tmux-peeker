package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ruleStyle paints every separator glyph — caps, fill, guards, label — in
// the theme's success green, bold, so the rules outrank any capture noise.
// Built per call: theme colors can change after package init.
func ruleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
}

// Rule glyphs for the interactive deck. Heavy mirrored caps bracket every
// rule; light guards hug the centered label.
const (
	ruleCapLeft     = "❯"
	ruleCapRight    = "❮"
	ruleGuardLeft   = "›"
	ruleGuardRight  = "‹"
	ruleHeroFill    = "━"
	ruleQueueFill   = "·"
	ruleMinSideFill = 3 // shortest fill run allowed beside a label
)

// heroHeaderRule opens the hero block: a heavy rule carrying the selected
// window's label. Fill and caps use the border color; the label assembly
// is muted.
func heroHeaderRule(width int, title string) string {
	return labeledRule(ruleHeroFill, title, width)
}

// queueRule is a band header: a dotted rule labeled with the window name
// of the band directly below it.
func queueRule(width int, title string) string {
	return labeledRule(ruleQueueFill, title, width)
}

// labeledRule renders `❯ fill › label ‹ fill ❮` across exactly width
// cells: one space at each joint, centered label assembly, truncated
// label when the window name cannot fit. Every cell — joints included —
// carries an explicit color.
func labeledRule(fill, label string, width int) string {
	label = truncateRuleLabel(label, width)
	style := ruleStyle()

	core := " " + ruleGuardLeft + " " + label + " " + ruleGuardRight + " "
	fills := max(0, width-
		ansi.StringWidth(ruleCapLeft)-ansi.StringWidth(ruleCapRight)-
		ansi.StringWidth(core)-2)
	left := fills / 2
	line := style.Render(ruleCapLeft+" "+strings.Repeat(fill, left)) +
		style.Render(core) +
		style.Render(strings.Repeat(fill, fills-left)+" "+ruleCapRight)
	return padOrTruncate(line, width)
}

// truncateRuleLabel shrinks a label so the rule still carries at least
// ruleMinSideFill fill glyphs on each side of the guard assembly.
func truncateRuleLabel(label string, width int) string {
	budget := width -
		ansi.StringWidth(ruleCapLeft) - ansi.StringWidth(ruleCapRight) -
		ansi.StringWidth(ruleGuardLeft) - ansi.StringWidth(ruleGuardRight) -
		4 - 2*ruleMinSideFill
	if budget < 1 {
		return ""
	}
	return ansi.Truncate(label, budget, "…")
}
