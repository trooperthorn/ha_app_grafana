package plugin

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The time-range macros. They expand to bound parameter references, never to literal
// text, so the dashboard's time range reaches SWIS as a typed parameter and the query
// plan is reused across refreshes. The names carry a double underscore so they cannot
// collide with a parameter a user binds.
const (
	paramTimeFrom = "__timeFrom"
	paramTimeTo   = "__timeTo"
)

var (
	// $__timeFilter(alias.Column): a column reference is letters, digits, underscores and
	// dots. Anything else is refused rather than pasted into the statement.
	timeFilterRE = regexp.MustCompile(`\$__timeFilter\(\s*([A-Za-z_][A-Za-z0-9_.]*)\s*\)`)
	timeFromRE   = regexp.MustCompile(`\$__timeFrom\(\s*\)`)
	timeToRE     = regexp.MustCompile(`\$__timeTo\(\s*\)`)
	badMacroRE   = regexp.MustCompile(`\$__\w+`)
)

// ExpandMacros rewrites the Grafana macros in a SWQL statement and reports which time
// parameters the result references, so the caller binds only what the query uses. SWIS
// rejects a bound parameter the statement does not mention.
func ExpandMacros(swql string) (string, map[string]bool, error) {
	used := map[string]bool{}
	out := timeFilterRE.ReplaceAllStringFunc(swql, func(m string) string {
		col := timeFilterRE.FindStringSubmatch(m)[1]
		used[paramTimeFrom], used[paramTimeTo] = true, true
		return fmt.Sprintf("%s >= @%s AND %s <= @%s", col, paramTimeFrom, col, paramTimeTo)
	})
	if timeFromRE.MatchString(out) {
		used[paramTimeFrom] = true
		out = timeFromRE.ReplaceAllString(out, "@"+paramTimeFrom)
	}
	if timeToRE.MatchString(out) {
		used[paramTimeTo] = true
		out = timeToRE.ReplaceAllString(out, "@"+paramTimeTo)
	}
	if m := badMacroRE.FindString(out); m != "" {
		return "", nil, fmt.Errorf("unknown or malformed macro %q; the supported macros are $__timeFilter(column), $__timeFrom() and $__timeTo()", m)
	}
	return out, used, nil
}

// swisTime formats a time the way SWIS binds a DateTime parameter: ISO 8601 in UTC. The
// history and statistics columns hold UTC (docs/swql/date-and-time.md), so comparing a
// UTC bound against them is the comparison that means what it says.
func swisTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// stripComments removes SQL line comments so a commented-out macro is not expanded and a
// commented-out parameter reference is not counted as used.
func stripComments(swql string) string {
	var b strings.Builder
	for _, line := range strings.Split(swql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
