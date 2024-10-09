package smtp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/k3a/html2text"
)

type SmtpSend struct {
	Host string
	User string
	Pass string
}

// The Backend implements SMTP server methods.
type Backend struct {
	db       db.DB
	SmtpSend
	to       string
	fromRe   *regexp.Regexp
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

var borrowRe = regexp.MustCompile(`^(?i)Borrowed[ +](.*)$`)
// Handle "Re:" and other localised versions
// TODO: Non-ASCII?
var aliasRe = regexp.MustCompile(`^(?i)(\w*:)?Alias\b`)

var InvalidError = errors.New("Invalid")

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Println("Mail from:", from)
	s.From = &from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Println("Rcpt to:", to)
	if to != s.Backend.to {
		return InvalidError
	}
	return nil
}

func (s *Session) Data(r io.Reader) error {
	m, err := mail.ReadMessage(r)
	if err != nil {
		return err
	}

	subject := m.Header.Get("Subject")
	if borrow := borrowRe.FindStringSubmatch(subject); borrow != nil {
		return s.processBorrow(borrow[1], m)
	} else if aliasRe.FindStringIndex(subject) != nil {
		return s.processAlias(m)
	} else {
		log.Println("Bad command", subject)
		return InvalidError
	}
}

// Process multipart emails, reading text if possible, otherwise converting
// HTML. Max input length given by buffer
func processEmail(contentType string, body io.Reader) (string, error) {
	buf := make([]byte, 10 * 1024)
	mediaType, params, err := mime.ParseMediaType(contentType)
	if mediaType != "multipart/alternative" {
		n, err := body.Read(buf)
		if n == 0 && err != nil {
			return "", err
		}

		if mediaType == "text/html" {
			return html2text.HTML2Text(string(buf[:n])), nil
		} else {
			return string(buf[:n]), nil
		}
	}
	if err != nil {
		return "", err
	}
	mr := multipart.NewReader(body, params["boundary"])
	var html, text string
	for {
		p, err := mr.NextPart()

		if err == io.EOF {
			if text != "" {
				return text, nil
			} else if html != "" {
				return html2text.HTML2Text(html), nil
			} else {
				return "", InvalidError
			}
		}

		n, err := p.Read(buf)
		if n == 0 && err != nil {
			return "", err
		}

		mediaType, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if mediaType == "text/plain" {
			text = strings.Clone(string(buf[:n]))
		}
		if mediaType == "text/html" {
			html = strings.Clone(string(buf[:n]))
		}
	}
}

func (s *Session) processBorrow(borrow string, m *mail.Message) error {
	body, err := processEmail(m.Header.Get("Content-Type"), m.Body)
	if err != nil {
		log.Println("Error", err.Error())
		return InvalidError
	}

	if s.Backend.fromRe.FindStringIndex(*s.From) == nil {
		go s.notifyAliasSetup(*s.From, s.Backend.to)
	}

	comment := strings.SplitN(body, "\n", 2)[0]
	location := db.Location{
		Tool:       borrow,
		LastSeenBy: *s.From,
		Comment:    &comment,
	}
	s.Backend.db.UpdateLocation(location)

	return nil
}

func (s *Session) processAlias(m *mail.Message) error {
	body, err := processEmail(m.Header.Get("Content-Type"), m.Body)
	if err != nil {
		log.Println("Error", err.Error())
		return InvalidError
	}

	alias := strings.SplitN(body, "\n", 2)[0]
	location := db.Alias{
		Email: *s.From,
		Alias: alias,
	}
	s.Backend.db.UpdateAlias(location)

	return nil
}

func (s *Session) notifyAliasSetup(to, from string) {
	if s.Backend.SmtpSend.Host == "" || s.Backend.SmtpSend.User == "" || s.Backend.SmtpSend.Pass == ""  {
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

func Serve(db db.DB, send SmtpSend, listen, domain, to string, fromRe *regexp.Regexp) {
	be := &Backend{db, send, to, fromRe}
	s := smtp.NewServer(be)

	s.Addr = listen
	s.Domain = domain
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
