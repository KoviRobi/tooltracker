package mail

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-msgauth/dkim"

	"github.com/KoviRobi/tooltracker/db"
	. "github.com/KoviRobi/tooltracker/test_utils"
)

var testPrivateKey *rsa.PrivateKey

func init() {
	block, _ := pem.Decode([]byte(TestPrivateKeyPEM))
	var err error
	testPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}

	verifyOptions.LookupTXT = func(query string) ([]string, error) {
		selector, _, ok := strings.Cut(query, "._domainkey.")
		if !ok {
			return nil, fmt.Errorf("Invalid dns/txt query To %s, missing `*._domainkey.*`", query)
		}
		switch selector {
		case "revoked":
			return []string{"v=DKIM1; p="}, nil
		default:
			return []string{
				"v=DKIM1; k=rsa; p=" +
					base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(
						&testPrivateKey.PublicKey)),
			}, nil
		}
	}
}

func sign(domain, selector, mail string) (string, error) {
	r := strings.NewReader(mail)
	options := &dkim.SignOptions{
		Domain:   domain,
		Selector: selector,
		Signer:   testPrivateKey,
	}
	var b bytes.Buffer
	err := dkim.Sign(&b, r, options)
	return b.String(), err
}

func newSigned(domain, selector, from, To, tool, body string) ([]byte, error) {
	plain := fmt.Sprintf(PlainTemplate, from, To, tool, body)
	crlf := strings.ReplaceAll(plain, "\n", "\r\n")
	signed, err := sign(domain, selector, crlf)
	return []byte(signed), err
}

func TestSigned(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	s.From = &User1
	msg, err := newSigned(Domain1, "valid", User1, To, Borrow+Tool1, "")
	Assert(t, err)
	Assert(t, s.Handle(msg))

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

func TestNotSigned(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	s.From = &User1
	err := s.Handle(newPlain(User1, To, Borrow+Tool1, ""))
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}

	items := conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)
}

func TestLocalNotSigned(t *testing.T) {
	conn, s := setup(t, Domain1, true, false)
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

func TestNoKey(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	s.From = &User1
	msg, err := newSigned(Domain1, "revoked", User1, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}

	items := conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)
}

func TestBadDomain(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	s.From = &User3
	msg, err := newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}

	items := conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)
}

func TestDelegate(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	// Alias a new user@domain
	s.From = &User1
	userAlias := "User alias"
	msg, err := newSigned(Domain1, "valid", User1, To, Alias+User3, userAlias)
	Assert(t, err)
	Assert(t, s.Handle(msg))

	items := conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(User3); delegate != User1 {
		t.Fatalf("Expecting delegate for %s To be %s, got %s", User3, User1, delegate)
	}

	// Use new user@domain
	s.From = &User3
	msg, err = newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	Assert(t, s.Handle(msg))

	items = conn.GetItems(nil)
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       Tool1,
				LastSeenBy: User3,
			},
			Alias: &userAlias,
		},
	}
	AssertSlicesEqual(t, expected, items)

	// Test that other users and domains still not valid
	s.From = &User4
	msg, err = newSigned(Domain2, "valid", User4, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}
	s.From = &User5
	msg, err = newSigned(Domain3, "valid", User5, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}
}

func TestNoDelegate(t *testing.T) {
	conn, s := setup(t, Domain1, false, true)
	defer conn.Close()

	// Alias a new user@domain
	s.From = &User1
	userAlias := "User alias"
	msg, err := newSigned(Domain1, "valid", User1, To, Alias+User3, userAlias)
	Assert(t, err)
	Assert(t, s.Handle(msg))

	items := conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(User3); delegate != User1 {
		t.Fatalf("Expecting delegate for %s To be %s, got %s", User3, User1, delegate)
	}

	// Use new user@domain
	s.From = &User3
	msg, err = newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}

	items = conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)

	// Test that other users and domains still not valid
	s.From = &User4
	msg, err = newSigned(Domain2, "valid", User4, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}
	s.From = &User5
	msg, err = newSigned(Domain3, "valid", User5, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}
}

func TestNoUnsignedDelegate(t *testing.T) {
	conn, s := setup(t, Domain1, true, false)
	defer conn.Close()

	// Alias a new user@domain -- unsigned
	s.From = &User1
	userAlias := "User alias"

	Assert(t, s.Handle(newPlain(User1, To, Alias+User3, userAlias)))

	items := conn.GetItems(nil)
	AssertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(User3); delegate != User1 {
		t.Fatalf("Expecting delegate for %s To be %s, got %s", User3, User1, delegate)
	}

	// Use signed user@domain
	s.From = &User3
	msg, err := newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	Assert(t, s.Handle(msg))

	// Use plain user@domain
	s.From = &User3
	err = s.Handle(newPlain(User3, To, Borrow+Tool1, ""))
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}

	items = conn.GetItems(nil)
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       Tool1,
				LastSeenBy: User3,
			},
			Alias: &userAlias,
		},
	}
	AssertSlicesEqual(t, expected, items)

	// Test that other users and domains still not valid
	s.From = &User4
	msg, err = newSigned(Domain2, "valid", User4, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}
	s.From = &User5
	msg, err = newSigned(Domain3, "valid", User5, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Handle(msg)
	if err != InvalidError {
		t.Fatalf("Expected %v, got %v", InvalidError, err)
	}
}
