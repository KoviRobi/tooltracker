package smtp

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/mail"
	"github.com/emersion/go-msgauth/dkim"
)

const testPrivateKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQCzhPzglJHjTp6Yf+slxL4wyh+Xmdh478MjKQpStLEB2EML+5oJ
N+5x/cKa6TxChqZGVwwJrUyKMXUmwO8GI74FNWtCCAs0oeV2TtUVchTJmwWolAX0
WqIc3A60pZiXtcmUfmtW/pZz6bKsTua4zY+SeHvdjsBECOuJ6RNQRJPj9wIDAQAB
AoGAa6U5BWnROR4xl3xNAq7A5PyuiPdliM8ske7QE9vpsBN/0LWkHhb90ji58q4c
xj97gP49Z6gVF2CkwQI70dCo5fDx7YBKb8qQU/DlPODK2KPny5rwaYkzxaN5rCdc
tQwTUQvxdNqKwmLe8Za/TQ9TT2+HUFhv8G5zn/UjINMg8PECQQDl75HpyudgCy7q
QP1TjQ78XGmZTSEBC8l6ZP0RslYBt/hkqOY6bwNCHbVO+vo5Zd/NF7cV2govO6EC
NcE0jqTDAkEAx95nNVn0ZTLYKMhGg5u3+FWhnVHHZGR4/LIEzgpr6T/XVW1vtf6T
fuJ+z/g6JsWbWxPlBJc27euXY7zvcmbAvQJAYGqx28A6f1qRJKd10oguxYGWwjLG
aSLhLFKWj8ohKH1VShhM2incyuecNG8nZ9QhIWYVXrNcW+v8GuohhwFdcwJADRn/
GfgzlQ6oLMQ0GxxyCs1SMsXRlDsh0y64Mels+XU94FO0JvHxKTgfp/JVnYUGkgnT
0WE4MJBo9BjGeXFS4QJBAJNVgYLMGwl07BRJSqkVLggq2Bwgjs04QjXXeaIE4/Wc
OjYm8GPa+wBvAsXqv47hcLfYgo6cyANVJW4L8fzBOGo=
-----END RSA PRIVATE KEY-----
`

var (
	testPrivateKey *rsa.PrivateKey
)

func init() {
	block, _ := pem.Decode([]byte(testPrivateKeyPEM))
	var err error
	testPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}

	mail.VerifyOptions.LookupTXT = func(query string) ([]string, error) {
		selector, _, ok := strings.Cut(query, "._domainkey.")
		if !ok {
			return nil, errors.New(fmt.Sprintf("Invalid dns/txt query to %s, missing `*._domainkey.*`", query))
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

func newSigned(domain, selector, from, to, tool, body string) (io.Reader, error) {
	plain := fmt.Sprintf(plainTemplate, from, to, tool, body)
	crlf := strings.ReplaceAll(plain, "\n", "\r\n")
	signed, err := sign(domain, selector, crlf)
	return strings.NewReader(signed), err
}

func TestSigned(t *testing.T) {
	conn, s := setup(t, domain1, true, true)
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	r, err := newSigned(domain1, "valid", user1, to, borrow+tool1, "")
	assert(t, err)
	assert(t, s.Data(r))

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

func TestNotSigned(t *testing.T) {
	conn, s := setup(t, domain1, true, true)
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	err := s.Data(newPlain(user1, to, borrow+tool1, ""))
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items := conn.GetItems()
	assertSlicesEqual(t, nil, items)
}

func TestLocalNotSigned(t *testing.T) {
	conn, s := setup(t, domain1, true, false)
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

func TestNoKey(t *testing.T) {
	conn, s := setup(t, domain1, true, true)
	defer conn.Close()

	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	r, err := newSigned(domain1, "revoked", user1, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items := conn.GetItems()
	assertSlicesEqual(t, nil, items)
}

func TestBadDomain(t *testing.T) {
	conn, s := setup(t, domain1, true, true)
	defer conn.Close()

	assert(t, s.Mail(user3, nil))
	assert(t, s.Rcpt(to, nil))
	r, err := newSigned(domain2, "valid", user3, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items := conn.GetItems()
	assertSlicesEqual(t, nil, items)
}

func TestDelegate(t *testing.T) {
	conn, s := setup(t, domain1, true, true)
	defer conn.Close()

	// Alias a new user@domain
	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	userAlias := "User alias"
	r, err := newSigned(domain1, "valid", user1, to, alias+user3, userAlias)
	assert(t, err)
	assert(t, s.Data(r))

	items := conn.GetItems()
	assertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(user3); delegate != user1 {
		t.Fatalf("Expecting delegate for %s to be %s, got %s", user3, user1, delegate)
	}

	// Use new user@domain
	assert(t, s.Mail(user3, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain2, "valid", user3, to, borrow+tool1, "")
	assert(t, err)
	assert(t, s.Data(r))

	items = conn.GetItems()
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       tool1,
				LastSeenBy: user3,
			},
			Alias: &userAlias,
		},
	}
	assertSlicesEqual(t, expected, items)

	// Test that other users and domains still not valid
	assert(t, s.Mail(user4, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain2, "valid", user4, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
	assert(t, s.Mail(user5, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain3, "valid", user5, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
}

func TestNoDelegate(t *testing.T) {
	conn, s := setup(t, domain1, false, true)
	defer conn.Close()

	// Alias a new user@domain
	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	userAlias := "User alias"
	r, err := newSigned(domain1, "valid", user1, to, alias+user3, userAlias)
	assert(t, err)
	assert(t, s.Data(r))

	items := conn.GetItems()
	assertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(user3); delegate != user1 {
		t.Fatalf("Expecting delegate for %s to be %s, got %s", user3, user1, delegate)
	}

	// Use new user@domain
	assert(t, s.Mail(user3, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain2, "valid", user3, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items = conn.GetItems()
	assertSlicesEqual(t, nil, items)

	// Test that other users and domains still not valid
	assert(t, s.Mail(user4, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain2, "valid", user4, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
	assert(t, s.Mail(user5, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain3, "valid", user5, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
}

func TestNoUnsignedDelegate(t *testing.T) {
	conn, s := setup(t, domain1, true, false)
	defer conn.Close()

	// Alias a new user@domain -- unsigned
	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	userAlias := "User alias"

	assert(t, s.Data(newPlain(user1, to, alias+user3, userAlias)))

	items := conn.GetItems()
	assertSlicesEqual(t, nil, items)
	if delegate := conn.GetDelegatedEmailFor(user3); delegate != user1 {
		t.Fatalf("Expecting delegate for %s to be %s, got %s", user3, user1, delegate)
	}

	// Use signed user@domain
	assert(t, s.Mail(user3, nil))
	assert(t, s.Rcpt(to, nil))
	r, err := newSigned(domain2, "valid", user3, to, borrow+tool1, "")
	assert(t, err)
	assert(t, s.Data(r))

	// Use plain user@domain
	assert(t, s.Mail(user3, nil))
	assert(t, s.Rcpt(to, nil))
	err = s.Data(newPlain(user3, to, borrow+tool1, ""))
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}

	items = conn.GetItems()
	expected := []db.Item{
		{
			Location: db.Location{
				Tool:       tool1,
				LastSeenBy: user3,
			},
			Alias: &userAlias,
		},
	}
	assertSlicesEqual(t, expected, items)

	// Test that other users and domains still not valid
	assert(t, s.Mail(user4, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain2, "valid", user4, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
	assert(t, s.Mail(user5, nil))
	assert(t, s.Rcpt(to, nil))
	r, err = newSigned(domain3, "valid", user5, to, borrow+tool1, "")
	assert(t, err)
	err = s.Data(r)
	if err != mail.InvalidError {
		t.Fatalf("Expected %v, got %v", mail.InvalidError, err)
	}
}
