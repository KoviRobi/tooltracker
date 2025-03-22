//go:build !odbc

package db

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	. "github.com/KoviRobi/tooltracker/tags"
	"github.com/KoviRobi/tooltracker/test_utils"
)

func ExecAssert(t *testing.T, db DB, query string, args ...any) sql.Result {
	t.Helper()
	res, err := db.Exec(query, args...)
	if err != nil {
		t.Errorf("Failed to execute query: %#q\n", err)
		t.Errorf("Query:\n> %s\n", strings.ReplaceAll(query, "\n", "\n> "))
		t.Errorf("Args: %#v\n", args)
		t.FailNow()
	}
	return res
}

func TestSql(t *testing.T) {
	db := CommonInit(t)

	ExecAssert(t, db, `INSERT INTO tracker VALUES('tool1', 'user1@com.com', NULL);`)
	ExecAssert(t, db, `INSERT INTO tracker VALUES('tool2', 'user1@com.com', 'Comment');`)
	ExecAssert(t, db, `INSERT INTO tracker VALUES('tool3', 'user2@com.com', NULL);`)

	ExecAssert(t, db, `INSERT INTO tool VALUES('tool1', NULL,'');`)
	ExecAssert(t, db, `INSERT INTO tool VALUES('tool2', NULL,'');`)
	ExecAssert(t, db, `INSERT INTO tool VALUES('tool3', NULL,'');`)

	ExecAssert(t, db, `INSERT INTO tags VALUES('tag1', 'tool1');`)
	ExecAssert(t, db, `INSERT INTO tags VALUES('tag2', 'tool1');`)
	ExecAssert(t, db, `INSERT INTO tags VALUES('tag2', 'tool2');`)
	ExecAssert(t, db, `INSERT INTO tags VALUES('tag3', 'tool2');`)
	ExecAssert(t, db, `INSERT INTO tags VALUES('tag1', 'tool3');`)
	ExecAssert(t, db, `INSERT INTO tags VALUES('tag3', 'tool3');`)

	filter := Tags{"tag1": Not, "tag2": All, "tag3": Any}
	items := db.GetItems(filter)
	comment := "Comment"
	expected := []Item{
		{
			Location: Location{Tool: "tool2", LastSeenBy: "user1@com.com", Comment: &comment},
			Tags:     &[]string{"tag2", "tag3"},
		},
	}
	toolCmp := func(a, b Item) int { return strings.Compare(a.Tool, b.Tool) }
	slices.SortFunc(items, toolCmp)
	slices.SortFunc(expected, toolCmp)
	test_utils.AssertSlicesEqual(t, expected, items)
}
