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
MIICXwIBAAKBgQDwIRP/UC3SBsEmGqZ9ZJW3/DkMoGeLnQg1fWn7/zYtIxN2SnFC
jxOCKG9v3b4jYfcTNh5ijSsq631uBItLa7od+v/RtdC2UzJ1lWT947qR+Rcac2gb
to/NMqJ0fzfVjH4OuKhitdY9tf6mcwGjaNBcWToIMmPSPDdQPNUYckcQ2QIDAQAB
AoGBALmn+XwWk7akvkUlqb+dOxyLB9i5VBVfje89Teolwc9YJT36BGN/l4e0l6QX
/1//6DWUTB3KI6wFcm7TWJcxbS0tcKZX7FsJvUz1SbQnkS54DJck1EZO/BLa5ckJ
gAYIaqlA9C0ZwM6i58lLlPadX/rtHb7pWzeNcZHjKrjM461ZAkEA+itss2nRlmyO
n1/5yDyCluST4dQfO8kAB3toSEVc7DeFeDhnC1mZdjASZNvdHS4gbLIA1hUGEF9m
3hKsGUMMPwJBAPW5v/U+AWTADFCS22t72NUurgzeAbzb1HWMqO4y4+9Hpjk5wvL/
eVYizyuce3/fGke7aRYw/ADKygMJdW8H/OcCQQDz5OQb4j2QDpPZc0Nc4QlbvMsj
7p7otWRO5xRa6SzXqqV3+F0VpqvDmshEBkoCydaYwc2o6WQ5EBmExeV8124XAkEA
qZzGsIxVP+sEVRWZmW6KNFSdVUpk3qzK0Tz/WjQMe5z0UunY9Ax9/4PVhp/j61bf
eAYXunajbBSOLlx4D+TunwJBANkPI5S9iylsbLs6NkaMHV6k5ioHBBmgCak95JGX
GMot/L2x0IYyMLAz6oLWh2hm7zwtb0CgOrPo1ke44hFYnfc=
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
	conn, s := setup(t, domain1)
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
	conn, s := setup(t, domain1)
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

func TestNoKey(t *testing.T) {
	conn, s := setup(t, domain1)
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
	conn, s := setup(t, domain1)
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
	conn, s := setup(t, domain1)
	defer conn.Close()

	// Alias a new user@domain
	assert(t, s.Mail(user1, nil))
	assert(t, s.Rcpt(to, nil))
	userAlias := "User alias"
	r, err := newSigned(domain1, "valid", user1, to, alias+" "+user3, userAlias)
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
