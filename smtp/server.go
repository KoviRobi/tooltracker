package smtp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/k3a/html2text"
	"github.com/mcnijman/go-emailaddress"
	"github.com/mnako/letters"
)

const multipartPrefix = "multipart/"
const contentType = "Content-Type"
const contentTransferEncoding = "Content-Transfer-Encoding"
const maxPartBytes = 10 * 1024
const maxMessageBytes = 1024 * 1024
const maxRecipients = 10
const writeTimeout = 10 * time.Second
const readTimeout = 10 * time.Second

var InvalidError = errors.New("Invalid")

var verifyOptions = dkim.VerifyOptions{
	LookupTXT: net.LookupTXT,
}

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

// Either parts is non-empty, or body contains the body
type MailPart struct {
	// Lower case mime type
	MimeType string
	Body     string
	Parts    []MailPart
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
var aliasRe = regexp.MustCompile(`^(?i)(\w*:\s*)?Alias([ +].*)?\b`)

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
	buf := make([]byte, maxMessageBytes)
	n, err := r.Read(buf)
	if n == 0 && err != nil {
		log.Println(err)
		return InvalidError
	}
	reader := bytes.NewReader(buf[:n])

	if s.From == nil {
		log.Println("No from in session")
		return InvalidError
	}

	delegate := s.Backend.Db.GetDelegatedEmailFor(*s.From)

	if s.Backend.Dkim != "" {
		dkimDomain := s.Backend.Dkim
		// At this point, we must have set an alias delegate using DKIM valid alias
		// command
		if *s.From != delegate {
			address, err := emailaddress.Parse(*s.From)
			if err != nil {
				log.Println(err)
				return InvalidError
			}
			dkimDomain = address.Domain
		}
		reader.Seek(0, io.SeekStart)
		verifications, err := dkim.VerifyWithOptions(reader, nil)
		if err != nil {
			log.Println(err)
			return InvalidError
		}

		verified := false
		for _, verification := range verifications {
			if verification != nil {
				verified = verified || (verification.Err == nil && verification.Domain == dkimDomain)
			}
		}
		if !verified {
			log.Println("Failed to verify message")
			return InvalidError
		}
	}

	reader.Seek(0, io.SeekStart)
	m, err := letters.ParseEmail(reader)
	if err != nil {
		log.Println(err)
		return InvalidError
	}

	subject := m.Headers.Subject
	body := m.Text
	if body == "" {
		body = html2text.HTML2Text(m.HTML)
	}
	if borrow := borrowRe.FindStringSubmatch(subject); borrow != nil {
		return s.processBorrow(body, borrow[1])
	} else if alias := aliasRe.FindStringSubmatch(subject); alias != nil {
		// Only set up delegates from the DKIM validated email, to prevent chains of
		// delegates
		var delegates *string
		if *s.From == delegate {
			delegates = &alias[2]
		}
		return s.processAlias(body, delegates)
	} else {
		log.Println("Bad command", subject)
		return InvalidError
	}
}
func (s *Session) processBorrow(body, borrow string) error {
	if s.Backend.FromRe.FindStringIndex(*s.From) == nil {
		go s.notifyAliasSetup(*s.From, s.Backend.To)
	}

	body = strings.TrimSpace(body)
	comment := strings.SplitN(body, "\n", 2)[0]
	comment = strings.TrimSpace(comment)
	location := db.Location{
		Tool:       borrow,
		LastSeenBy: *s.From,
		Comment:    &comment,
	}
	s.Backend.Db.UpdateLocation(location)

	return nil
}

func (s *Session) processAlias(body string, delegateFrom *string) error {
	body = strings.TrimSpace(body)
	alias := strings.SplitN(body, "\n", 2)[0]
	alias = strings.TrimSpace(alias)
	s.Backend.Db.UpdateAlias(db.Alias{
		Email: *s.From,
		Alias: alias,
	})

	if delegateFrom != nil {
		from := emailaddress.FindWithRFC5322([]byte(*delegateFrom), false)
		for _, address := range from {
			s.Backend.Db.UpdateAlias(db.Alias{
				Email: address.String(),
				Alias: alias,
				DelegatedEmail: s.From,
			})
		}
	}

	return nil
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
