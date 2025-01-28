package smtp

import (
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/limits"
)

const to = "tooltracker@a.example.com"
const domain1 = "a.example.com"
const domain2 = "b.example.com"
const domain3 = "c.example.com"
const tool1 = "tool1"
const tool2 = "tool2"

var user1 = fmt.Sprintf("user1@%s", domain1)
var user2 = fmt.Sprintf("user2@%s", domain1)
var user3 = fmt.Sprintf("user3@%s", domain2)
var user4 = fmt.Sprintf("user4@%s", domain2)
var user5 = fmt.Sprintf("user5@%s", domain3)
var fromRe = regexp.MustCompile(fmt.Sprintf(".*@%s", regexp.QuoteMeta(domain1)))

const borrow = "Borrowed "
const alias = "Alias "
const plainTemplate = `From: %s
To: %s
Subject: %s

%s
`

func init() {
	limits.MaxMessageBytes = 1024
	limits.MaxRecipients = 1
}

func newMailStringReader(s string) io.Reader {
	return strings.NewReader(strings.ReplaceAll(s, "\n", "\r\n"))
}

func newPlain(from, to, tool, body string) io.Reader {
	return newMailStringReader(fmt.Sprintf(plainTemplate, from, to, tool, body))
}

func setup(t *testing.T, dkim string) (db.DB, Session) {
	conn, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	err = conn.EnsureTooltrackerTables()
	if err != nil {
		t.Fatal(err)
	}

	be := Backend{
		Db:     conn,
		To:     to,
		FromRe: fromRe,
		Dkim:   dkim,
	}

	s := Session{
		Backend: &be,
	}

	if conn.GetItems() != nil {
		t.Fatalf("Expected DB to be empty at start")
	}

	return conn, s
}

func assert(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertSlicesEqual[T fmt.Stringer](t *testing.T, expected []T, got []T) {
	t.Helper()
	if !reflect.DeepEqual(expected, got) {
		error := "Expected:\n\t"
		for _, item := range expected {
			error += strings.ReplaceAll(item.String(), "\n", "\n\t")
		}
		error += "Got:\n\t"
		for _, item := range got {
			error += strings.ReplaceAll(item.String(), "\n", "\n\t")
		}
		t.Fatal(error)
	}
}

func TestBorrowed(t *testing.T) {
	conn, s := setup(t, "")
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	assert(t, s.Data(newPlain(user1, to, borrow+tool1, "")))

	items := conn.GetItems()
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       tool1,
				LastSeenBy: user1,
			},
		},
	}
	assertSlicesEqual(t, expected, items)
}

func TestBorrowedPlain(t *testing.T) {
	conn, s := setup(t, "")
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	comment := "Some comment"
	assert(t, s.Data(newPlain(user1, to, borrow+tool1, comment)))

	items := conn.GetItems()
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       tool1,
				LastSeenBy: user1,
				Comment:    &comment,
			},
		}}
	assertSlicesEqual(t, expected, items)
}

func TestBorrowedHTML(t *testing.T) {
	conn, s := setup(t, "")
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))

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
`, user1, to, tool1, comment)
	assert(t, s.Data(newMailStringReader(eml)))

	items := conn.GetItems()
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       tool1,
				LastSeenBy: user1,
				Comment:    &comment,
			},
		},
	}
	assertSlicesEqual(t, expected, items)
}

func TestBorrowedUpdate(t *testing.T) {
	conn, s := setup(t, "")
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	assert(t, s.Data(newPlain(user1, to, borrow+tool1, "")))

	assert(t, s.Mail(user2, nil))
	assert(t, s.Rcpt(to, nil))
	assert(t, s.Data(newPlain(user2, to, borrow+tool1, "")))

	items := conn.GetItems()
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       tool1,
				LastSeenBy: user2,
			},
		},
	}
	assertSlicesEqual(t, expected, items)
}

func TestBorrowedMultiple(t *testing.T) {
	conn, s := setup(t, "")
	defer conn.Close()

	err := s.Mail(user1, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Data(newPlain(user1, to, borrow+tool1, ""))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Mail(user2, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Data(newPlain(user2, to, borrow+tool2, ""))
	if err != nil {
		t.Fatal(err)
	}

	items := conn.GetItems()
	expected1 := db.Item{
		Location: db.Location{
			Tool:       tool1,
			LastSeenBy: user1,
		},
	}
	expected2 := db.Item{
		Location: db.Location{
			Tool:       tool2,
			LastSeenBy: user2,
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
