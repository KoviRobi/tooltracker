package smtp

import (
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/KoviRobi/tooltracker/db"
)
const to = "tooltracker@a.example.com"
const fromDomain = "b.example.com"
const tool1 = "tool1"
const tool2 = "tool2"
var user1 = fmt.Sprintf("user1@%s", fromDomain)
var user2 = fmt.Sprintf("user2@%s", fromDomain)
var fromRe = regexp.MustCompile(fmt.Sprintf(".*@%s", regexp.QuoteMeta(fromDomain)))

const plainBorrowTemplate = `From: %s
To: %s
Subject: Borrowed %s

%s
`

func newMailStringReader(s string) io.Reader {
	return strings.NewReader(strings.Replace(s, "\n", "\r\n", -1))
}

func setup(t *testing.T) (db.DB, Session) {
	conn, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}

	be := Backend{
		Db: conn,
		To: to,
		FromRe: fromRe,
	}

	s := Session{
		Backend: &be,
	}

	if conn.GetItems() != nil {
		t.Fatalf("Expected DB to be empty at start")
	}

	return conn, s
}

func TestBorrowedSimple(t *testing.T) {
	conn, s := setup(t)

	err := s.Mail(user1, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Data(newMailStringReader(fmt.Sprintf(plainBorrowTemplate, user1, to, tool1, "")))
	if err != nil {
		t.Fatal(err)
	}

	items := conn.GetItems()
	expected := db.Item{
		Location: db.Location{
			Tool: tool1,
			LastSeenBy: user1,
		},
	}
	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}
	if items[0] != expected {
		t.Fatalf("Expected %v, got %v\n", expected.String(), items[0].String())
	}
}

func TestBorrowedSimpleUpdate(t *testing.T) {
	conn, s := setup(t)

	err := s.Mail(user1, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Data(newMailStringReader(fmt.Sprintf(plainBorrowTemplate, user1, to, tool1, "")))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Mail(user2, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Data(newMailStringReader(fmt.Sprintf(plainBorrowTemplate, user2, to, tool1, "")))
	if err != nil {
		t.Fatal(err)
	}

	items := conn.GetItems()
	expected := db.Item{
		Location: db.Location{
			Tool: tool1,
			LastSeenBy: user2,
		},
	}
	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}
	if items[0] != expected {
		t.Fatalf("Expected %v, got %v\n", expected.String(), items[0].String())
	}
}

func TestBorrowedSimple2(t *testing.T) {
	conn, s := setup(t)

	err := s.Mail(user1, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Data(newMailStringReader(fmt.Sprintf(plainBorrowTemplate, user1, to, tool1, "")))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Mail(user2, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.Rcpt(to, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Data(newMailStringReader(fmt.Sprintf(plainBorrowTemplate, user2, to, tool2, "")))
	if err != nil {
		t.Fatal(err)
	}

	items := conn.GetItems()
	expected1 := db.Item{
		Location: db.Location{
			Tool: tool1,
			LastSeenBy: user1,
		},
	}
	expected2 := db.Item{
		Location: db.Location{
			Tool: tool2,
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
