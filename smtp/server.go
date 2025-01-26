package smtp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/mail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

const maxMessageBytes = 1024 * 1024
const maxRecipients = 10
const writeTimeout = 10 * time.Second
const readTimeout = 10 * time.Second

var InvalidToError = errors.New("Invalid 'to' in envelope")

type SmtpSend struct {
	Host string
	User string
	Pass string
}

// The Backend implements SMTP server methods.
type Backend struct {
	SmtpSend
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
		return InvalidToError
	}
	return nil
}

func (s *Session) Data(r io.Reader) error {
	mailSession := mail.Session{
		Db:   s.Backend.Db,
		Dkim: s.Backend.Dkim,
		From: s.From,
	}
	return mailSession.Handle(r)
}

func (s *Session) notifyAliasSetup(to, from string) {
	if s.Backend.SmtpSend.Host == "" || s.Backend.SmtpSend.User == "" || s.Backend.SmtpSend.Pass == "" {
		return
	}

	body := fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: Alias\r\n"+
		"X-Tooltracker-Type: Alias\r\n"+
		"\r\n"+
		"Your email isn't a work e-mail. Reply to this e-mail with the first\r\n"+
		"line of the reply containing an alias you wish to use (to hide your\r\n"+
		"personal e-mail address).",
		to,
		from,
	)
	auth := sasl.NewPlainClient("", s.Backend.SmtpSend.User, s.Backend.SmtpSend.Pass)
	err := smtp.SendMail(s.Backend.SmtpSend.Host, auth, from, []string{to}, strings.NewReader(body))
	if err != nil {
		log.Printf("Failed to send alias notification from %#v to %#v body %s err %v %v\n",
			from, to, body, err, err.Error())
	}
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
