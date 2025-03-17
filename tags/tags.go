package tags

import (
	"regexp"
)

// Note: left-most regexp means we match the "-" if it is present
var Re = regexp.MustCompile(`[+-]?[a-zA-Z][a-zA-Z0-9_]*`)

const Hidden = `hidden`
const DefaultFilter = `-hidden`

// Filters according to OR-ing positive tags (not beginning with '-'), removing
// negative tags (beginning with '-')
func TagsSqlFilter(tags []string) (string, []any) {
	anyFilter := ``
	allFilter := ``
	allCount := 0
	notFilter := ``

	var args []any
	var anyArgs []any
	var allArgs []any
	var notArgs []any

	// Make a table of tools that have matching tags
	matchTable := `SELECT tool.name FROM tool
	LEFT JOIN tags ON tool.name = tags.tool
	WHERE tags.tag IN (`
	// Any have matching tags
	anySep := `tracker.tool IN (` + matchTable
	// Any matches, but then enforcing COUNT below
	allSep := `tracker.tool IN (` + matchTable
	// No matching tags in `not` table
	notSep := `tracker.tool NOT IN (` + matchTable

	for _, tag := range tags {
		if tag[0] == '-' {
			notArgs = append(notArgs, tag[1:])
			notFilter += notSep + `?`
			notSep = `, `
		} else {
			if tag[0] == '+' {
				tag = tag[1:]
				allArgs = append(allArgs, tag)
				allFilter += allSep + `?`
				allSep = `, `
				allCount++
			}
			anyArgs = append(anyArgs, tag)
			anyFilter += anySep + `?`
			anySep = `, `
		}
	}

	// Note, closing parentheses match up with `(any|all|not)Sep` vars
	filter := ``
	if anyFilter != `` {
		filter += anyFilter + `))`
		args = append(args, anyArgs...)
	}
	if allFilter != `` {
		if filter != `` {
			filter += ` AND `
		}
		filter += allFilter + `) GROUP BY tags.tool HAVING COUNT(tags.tag) = ?)`
		args = append(append(args, allArgs...), allCount)
	}
	if notFilter != `` {
		if filter != `` {
			filter += ` AND `
		}
		filter += notFilter + `))`
		args = append(args, notArgs...)
	}

	return filter, args
}
