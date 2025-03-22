package tags

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
)

// Note: left-most regexp means we match the "-" if it is present
var Re = regexp.MustCompile(`[+-]?[a-zA-Z][a-zA-Z0-9_]*`)

const Hidden = `hidden`

type TagType string

// Ordered in which one has highest precedence
const (
	Not TagType = "-"
	All TagType = "+"
	Any TagType = ""
)

type Tags map[string]TagType

func (tags Tags) String() string {
	ret := ""
	sep := ""
	for tag, tagType := range tags {
		ret += sep + string(tagType) + tag
		sep = " "
	}
	return ret
}

var DefaultFilter = Tags{Hidden: Not}

// Parse tag into type (-/+/none) and body
func ParseTag(tag string) (string, TagType) {
	if len(tag) > 0 {
		switch tag[0] {
		case '-':
			return tag[1:], Not
		case '+':
			return tag[1:], All
		}
	}
	return tag, Any
}

// Becaue the array might be from e.g. tags=foo+bar&tags=baz, first join, then
// split again. Then remove duplicates, have "-tag" remove "+tag" and "tag",
// have "+tag" remove "tag"
func NormalizeTags(tagsSlice []string) Tags {
	tagsSlice = Re.FindAllString(strings.Join(tagsSlice, " "), -1)
	ret := make(Tags, len(tagsSlice))
	for _, tag := range tagsSlice {
		body, new := ParseTag(tag)
		old, found := ret[body]
		if !found {
			old = new
		}
		// -tag takes predecence over +tag and tag
		// +tag takes precedence over tag
		ret[body] = min(old, new)
	}
	return ret
}

// Add tags and return as a query string
func AddTag(tags Tags, tag string) string {
	body, new := ParseTag(tag)
	tags = maps.Clone(tags)
	tags[body] = new
	return tags.String()
}

func DelTag(tags Tags, tag string) string {
	body, new := ParseTag(tag)
	tags = maps.Clone(tags)
	old, found := tags[body]
	if found && old == new {
		delete(tags, body)
	}
	return tags.String()
}

func joinRepeat(s, sep string, n int) string {
	sep1 := ""
	ret := ""
	for range n {
		ret += sep1 + s
		sep1 = sep
	}
	return ret
}

// Filters according to OR-ing positive tags (not beginning with '-'), removing
// negative tags (beginning with '-')
func TagsSqlFilter(tags Tags) (string, []any) {
	var args []any
	var anyArgs []any
	var allArgs []any
	var notArgs []any

	// Make a table of tools that have matching tags
	matchTable := `
	SELECT tool.name FROM tool
	LEFT JOIN tags ON tool.name = tags.tool
	WHERE tags.tag IN (%s)`

	for tag, tagType := range tags {
		if tagType == Not {
			notArgs = append(notArgs, tag)
		} else {
			if tagType == All {
				allArgs = append(allArgs, tag)
			}
			anyArgs = append(anyArgs, tag)
		}
	}

	filter := ``
	sep := `WHERE`
	if anyArgs != nil {
		filter += fmt.Sprintf(
			` %s tracker.tool IN (%s)`,
			sep,
			fmt.Sprintf(matchTable, joinRepeat("?", ",", len(anyArgs))))
		sep = ` AND `
		args = append(args, anyArgs...)
	}
	if allArgs != nil {
		filter += fmt.Sprintf(
			` %s tracker.tool IN (%s GROUP BY tags.tool HAVING count(tags.tag) = ?)`,
			sep,
			fmt.Sprintf(matchTable, joinRepeat("?", ",", len(allArgs))))
		sep = ` AND `
		args = append(args, allArgs...)
		args = append(args, len(allArgs))
	}
	if notArgs != nil {
		filter += fmt.Sprintf(
			` %s tracker.tool NOT IN (%s)`,
			sep,
			fmt.Sprintf(matchTable, joinRepeat("?", ",", len(notArgs))))
		sep = ` AND `
		args = append(args, notArgs...)
	}

	return filter, args
}
