package smtp

import (
	"errors"
	"io"
	"log"
	"net/mail"
	"regexp"
	"time"

	"github.com/emersion/go-smtp"
)

// The Backend implements SMTP server methods.
type Backend struct{}

// NewSession is called after client greeting (EHLO, HELO).
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{Backend: bkd}, nil
}

// A Session is returned after successful login.
type Session struct {
	Backend *Backend
	From    *string
	To      *string
}

// TODO:  Don't hardcode
var fromRe = regexp.MustCompile("^.*@carallon.com$|^.*@user-mail.com$")
var toRe = regexp.MustCompile("^tooltracker@.*$")
var borrowRe = regexp.MustCompile(`^Borrowed (.*)$`)

var InvalidError = errors.New("Invalid")

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Println("Mail from:", from)
	s.From = &from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Println("Rcpt to:", to)
	s.To = &to
	return nil
}

func (s *Session) Data(r io.Reader) error {
	m, err := mail.ReadMessage(r)
	if err != nil {
		return err
	}

	subject := m.Header.Get("Subject")
	borrow := borrowRe.FindStringSubmatch(subject)
	if borrow != nil {
		log.Println("Borrow", borrow)
	} else {
		log.Println("Bad borrow", subject)
		return InvalidError
	}

	body, err := io.ReadAll(m.Body)
	if err != nil {
		log.Println("Error", err.Error())
		return InvalidError
	}

	if s.From == nil || fromRe.FindString(*s.From) == "" {
		log.Println("Bad from", *s.From)
		return InvalidError
	}

	if s.To == nil || toRe.FindString(*s.To) == "" {
		log.Println("Bad to", *s.To)
		return InvalidError
	}

	log.Printf(
		"Updating location of %#v to last seen by %#v, comment %#v\n",
		borrow[1],
		*s.From, string(body),
	)

	return nil
}

func (s *Session) Reset() {}

func (s *Session) Logout() error {
	return nil
}

func Serve() {
	be := &Backend{}

	s := smtp.NewServer(be)

	// TODO: Don't hardcode
	s.Addr = "localhost:1025"
	s.Domain = "0.0.0.0"
	s.WriteTimeout = 10 * time.Second
	s.ReadTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
