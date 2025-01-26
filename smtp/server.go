package smtp

import (
	"errors"
	"io"
	"log"
	"regexp"
	"time"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/mail"
	"github.com/emersion/go-smtp"
)

const maxMessageBytes = 1024 * 1024
const maxRecipients = 10
const writeTimeout = 10 * time.Second
const readTimeout = 10 * time.Second

var InvalidError = errors.New("Invalid SMTP envelope")

// The Backend implements SMTP server methods.
type Backend struct {
	Db     db.DB
	To     string
	Dkim   string
	FromRe *regexp.Regexp
}

// NewSession is called after client greeting (EHLO, HELO).
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{Backend: bkd}, nil
}

// A Session is returned after successful login.
type Session struct {
	Backend *Backend
	From    *string
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Println("Mail from:", from)
	s.From = &from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Println("Rcpt to:", to)
	if to != s.Backend.To {
		log.Println("Expecting rcpt to:", s.Backend.To)
		return InvalidError
	}
	return nil
}

func (s *Session) Data(r io.Reader) error {
	mailSession := mail.Session{
		Db:   s.Backend.Db,
		Dkim: s.Backend.Dkim,
		From: s.From,
	}
	buf := make([]byte, maxMessageBytes)
	n, err := r.Read(buf)
	if n == 0 && err != nil {
		log.Printf("Error reading mail from reader: %v", err)
		return InvalidError
	}
	return mailSession.Handle(buf[:n])
}

func (s *Session) Reset() {}

func (s *Session) Logout() error {
	return nil
}

func Serve(listen, domain string, backend Backend) {
	s := smtp.NewServer(&backend)

	s.Addr = listen
	s.Domain = domain
	s.WriteTimeout = writeTimeout
	s.ReadTimeout = readTimeout
	s.MaxMessageBytes = maxMessageBytes
	s.MaxRecipients = maxRecipients
	s.AllowInsecureAuth = true

	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
