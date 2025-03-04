package mail

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/KoviRobi/tooltracker/db"
	. "github.com/KoviRobi/tooltracker/test_utils"
)

func newPlain(from, to, Tool, body string) []byte {
	return []byte(fmt.Sprintf(PlainTemplate, from, to, Tool, body))
}

func setup(t *testing.T, dkim string, delegate, localDkim bool) (db.DB, Session) {
	conn := CommonInit(t)

	s := Session{
		Db:        conn,
		Dkim:      dkim,
		Delegate:  delegate,
		LocalDkim: localDkim,
	}

	return conn, s
}

func TestBorrowed(t *testing.T) {
	conn, s := setup(t, "", true, true)
	defer conn.Close()

	s.From = &User1
	Assert(t, s.Handle(newPlain(User1, To, Borrow+Tool1, "")))

	items := conn.GetItems(nil)
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       Tool1,
				LastSeenBy: User1,
			},
		},
	}
	AssertSlicesEqual(t, expected, items)
}

func TestBorrowedPlain(t *testing.T) {
	conn, s := setup(t, "", true, true)
	defer conn.Close()

	s.From = &User1
	comment := "Some comment"
	Assert(t, s.Handle(newPlain(User1, To, Borrow+Tool1, comment)))

	items := conn.GetItems(nil)
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       Tool1,
				LastSeenBy: User1,
				Comment:    &comment,
			},
		},
	}
	AssertSlicesEqual(t, expected, items)
}

func TestBorrowedHTML(t *testing.T) {
	conn, s := setup(t, "", true, true)
	defer conn.Close()

	s.From = &User1

	comment := "Some comment"
	eml := fmt.Sprintf(`From: %s
To: %s
Subject: Borrowed %s
Content-Type: text/html; charset="utf-8"

<html>
	<head></head>
	<body>
		<p>
			%s
		</p>
	</body>
</html>
`, User1, To, Tool1, comment)
	Assert(t, s.Handle([]byte(eml)))

	items := conn.GetItems(nil)
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       Tool1,
				LastSeenBy: User1,
				Comment:    &comment,
			},
		},
	}
	AssertSlicesEqual(t, expected, items)
}

func TestBorrowedUpdate(t *testing.T) {
	conn, s := setup(t, "", true, true)
	defer conn.Close()

	s.From = &User1
	Assert(t, s.Handle(newPlain(User1, To, Borrow+Tool1, "")))

	s.From = &User2
	Assert(t, s.Handle(newPlain(User2, To, Borrow+Tool1, "")))

	items := conn.GetItems(nil)
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       Tool1,
				LastSeenBy: User2,
			},
		},
	}
	AssertSlicesEqual(t, expected, items)
}

func TestBorrowedMultiple(t *testing.T) {
	conn, s := setup(t, "", true, true)
	defer conn.Close()

	s.From = &User1
	Assert(t, s.Handle(newPlain(User1, To, Borrow+Tool1, "")))

	s.From = &User2
	Assert(t, s.Handle(newPlain(User2, To, Borrow+Tool2, "")))

	items := conn.GetItems(nil)
	expected1 := db.Item{
		Location: db.Location{
			Tool:       Tool1,
			LastSeenBy: User1,
		},
	}
	expected2 := db.Item{
		Location: db.Location{
			Tool:       Tool2,
			LastSeenBy: User2,
		},
	}
	expected := map[db.Item]bool{expected1: true, expected2: true}
	got := make(map[db.Item]bool)
	for _, item := range items {
		got[item] = true
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected %v, got %v\n", expected, got)
	}
}
