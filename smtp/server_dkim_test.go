package smtp

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/emersion/go-msgauth/dkim"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/mail"
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

	mail.VerifyOptions.LookupTXT = func(query string) ([]string, error) {
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

func newSigned(domain, selector, from, To, tool, body string) (io.Reader, error) {
	plain := fmt.Sprintf(PlainTemplate, from, To, tool, body)
	crlf := strings.ReplaceAll(plain, "\n", "\r\n")
	signed, err := sign(domain, selector, crlf)
	return strings.NewReader(signed), err
}

func TestSigned(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err := newSigned(Domain1, "valid", User1, To, Borrow+Tool1, "")
	Assert(t, err)
	Assert(t, s.Data(r))

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

func TestNotSigned(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	err := s.Data(newPlain(User1, To, Borrow+Tool1, ""))
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items := conn.GetItems()
	AssertSlicesEqual(t, nil, items)
}

func TestLocalNotSigned(t *testing.T) {
	conn, s := setup(t, Domain1, true, false)
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

func TestNoKey(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err := newSigned(Domain1, "revoked", User1, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items := conn.GetItems()
	AssertSlicesEqual(t, nil, items)
}

func TestBadDomain(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	Assert(t, s.Mail(User3, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err := newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items := conn.GetItems()
	AssertSlicesEqual(t, nil, items)
}

func TestDelegate(t *testing.T) {
	conn, s := setup(t, Domain1, true, true)
	defer conn.Close()

	// Alias a new user@domain
	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	userAlias := "User alias"
	r, err := newSigned(Domain1, "valid", User1, To, Alias+User3, userAlias)
	Assert(t, err)
	Assert(t, s.Data(r))

	items := conn.GetItems()
	AssertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(User3); delegate != User1 {
		t.Fatalf("Expecting delegate for %s To be %s, got %s", User3, User1, delegate)
	}

	// Use new user@domain
	Assert(t, s.Mail(User3, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	Assert(t, s.Data(r))

	items = conn.GetItems()
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
	Assert(t, s.Mail(User4, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain2, "valid", User4, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
	Assert(t, s.Mail(User5, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain3, "valid", User5, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
}

func TestNoDelegate(t *testing.T) {
	conn, s := setup(t, Domain1, false, true)
	defer conn.Close()

	// Alias a new user@domain
	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	userAlias := "User alias"
	r, err := newSigned(Domain1, "valid", User1, To, Alias+User3, userAlias)
	Assert(t, err)
	Assert(t, s.Data(r))

	items := conn.GetItems()
	AssertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(User3); delegate != User1 {
		t.Fatalf("Expecting delegate for %s To be %s, got %s", User3, User1, delegate)
	}

	// Use new user@domain
	Assert(t, s.Mail(User3, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items = conn.GetItems()
	AssertSlicesEqual(t, nil, items)

	// Test that other users and domains still not valid
	Assert(t, s.Mail(User4, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain2, "valid", User4, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
	Assert(t, s.Mail(User5, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain3, "valid", User5, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
}

func TestNoUnsignedDelegate(t *testing.T) {
	conn, s := setup(t, Domain1, true, false)
	defer conn.Close()

	// Alias a new user@domain -- unsigned
	Assert(t, s.Mail(User1, nil))
	Assert(t, s.Rcpt(To, nil))
	userAlias := "User alias"

	Assert(t, s.Data(newPlain(User1, To, Alias+User3, userAlias)))

	items := conn.GetItems()
	AssertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(User3); delegate != User1 {
		t.Fatalf("Expecting delegate for %s To be %s, got %s", User3, User1, delegate)
	}

	// Use signed user@domain
	Assert(t, s.Mail(User3, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err := newSigned(Domain2, "valid", User3, To, Borrow+Tool1, "")
	Assert(t, err)
	Assert(t, s.Data(r))

	// Use plain user@domain
	Assert(t, s.Mail(User3, nil))
	Assert(t, s.Rcpt(To, nil))
	err = s.Data(newPlain(User3, To, Borrow+Tool1, ""))
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items = conn.GetItems()
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
	Assert(t, s.Mail(User4, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain2, "valid", User4, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
	Assert(t, s.Mail(User5, nil))
	Assert(t, s.Rcpt(To, nil))
	r, err = newSigned(Domain3, "valid", User5, To, Borrow+Tool1, "")
	Assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
}
