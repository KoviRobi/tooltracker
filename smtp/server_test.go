package smtp

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/KoviRobi/tooltracker/db"
	. "github.com/KoviRobi/tooltracker/test_utils"
)

func newMailStringReader(s string) io.Reader {
	return strings.NewReader(strings.ReplaceAll(s, "\n", "\r\n"))
}

func newPlain(from, to, Tool, body string) io.Reader {
	return newMailStringReader(fmt.Sprintf(PlainTemplate, from, to, Tool, body))
}

func setup(t *testing.T, dkim string, delegate, localDkim bool) (db.DB, Session) {
	conn := CommonInit(t)

	be := Backend{
		Db:        conn,
		To:        To,
		FromRe:    FromRe,
		Dkim:      dkim,
		Delegate:  delegate,
		LocalDkim: localDkim,
	}

	s := Session{
		Backend: &be,
	}

	return conn, s
}

func TestBorrowed(t *testing.T) {
	conn, s := setup(t, "", true, true)
	defer conn.Close()

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	Assert(t, s.Data(newPlain(User1, To, Borrow+Tool1, "")))

	items := conn.GetItems()
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

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	comment := "Some comment"
	Assert(t, s.Data(newPlain(User1, To, Borrow+Tool1, comment)))

	items := conn.GetItems()
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

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))

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
	Assert(t, s.Data(newMailStringReader(eml)))

	items := conn.GetItems()
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

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	Assert(t, s.Data(newPlain(User1, To, Borrow+Tool1, "")))

	Assert(t, s.Mail(User2, nil))
	Assert(t, s.Rcpt(To, nil))
	Assert(t, s.Data(newPlain(User2, To, Borrow+Tool1, "")))

	items := conn.GetItems()
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

	err := s.Mail(User1, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Rcpt(To, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Data(newPlain(User1, To, Borrow+Tool1, ""))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Mail(User2, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Rcpt(To, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Data(newPlain(User2, To, Borrow+Tool2, ""))
	if err != nil {
		t.Fatal(err)
	}

	items := conn.GetItems()
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
