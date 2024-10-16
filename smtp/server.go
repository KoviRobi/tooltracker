package smtp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/k3a/html2text"
)

const multipartPrefix = "multipart/"
const contentType = "Content-Type"
const contentTransferEncoding = "Content-Transfer-Encoding"
const maxPartBytes = 10 * 1024
const maxMessageBytes = 1024 * 1024

var InvalidError = errors.New("Invalid")

type SmtpSend struct {
	Host string
	User string
	Pass string
}

// The Backend implements SMTP server methods.
type Backend struct {
	db db.DB
	SmtpSend
	to     string
	fromRe *regexp.Regexp
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
var aliasRe = regexp.MustCompile(`^(?i)(\w*:)?Alias\b`)

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

func processTransferEncoding(encoding string, body []byte) (string, error) {
	encoding = strings.ToLower(encoding)
	if encoding == "base64" {
		buf := make([]byte, base64.StdEncoding.DecodedLen(len(body)))
		n, err := base64.StdEncoding.Decode(buf, body)
		if n == 0 && err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	} else if encoding == "quoted-printable" {
		buf := make([]byte, maxPartBytes)
		n, err := quotedprintable.NewReader(bytes.NewBuffer(body)).Read(buf)
		if n == 0 && err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	} else {
		// Assume plain
		return string(body), nil
	}
}

// Process multipart emails, reading text if possible, otherwise converting
// HTML. Max input length given by buffer
func processMultipart(mediaType string, encoding string, body io.Reader) (*MailPart, error) {
	buf := make([]byte, maxPartBytes)
	if contentType == "" {
		n, err := body.Read(buf)
		if n == 0 && err != nil {
			return nil, err
		}
		body, err := processTransferEncoding(encoding, buf)
		if err != nil {
			return nil, err
		}
		return &MailPart{Body: body}, nil
	}
	mediaType, params, err := mime.ParseMediaType(mediaType)
	// mediaType is now lower-case
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(mediaType, multipartPrefix) {
		mr := multipart.NewReader(body, params["boundary"])
		var parts []MailPart
		var partErr error
		for {
			p, err := mr.NextPart()

			if err == io.EOF {
				if parts == nil && partErr != nil {
					return nil, partErr
				}
				return &MailPart{MimeType: mediaType, Parts: parts}, nil
			}

			var part *MailPart
			part, partErr = processMultipart(
				p.Header.Get(contentType),
				p.Header.Get(contentTransferEncoding),
				p,
			)

			if part != nil {
				parts = append(parts, *part)
			}
		}
	} else {
		n, err := body.Read(buf)
		if n == 0 && err != nil {
			return nil, err
		}
		body, err := processTransferEncoding(encoding, buf)
		if err != nil {
			return nil, err
		}
		return &MailPart{MimeType: mediaType, Body: body}, nil
	}
}

// Extract the text part of the e-mail from `*MailPart, err` produced by
// processMultipart. Not perfect but hopefully covers the encountered cases.
func getTextPart(parts *MailPart, err error) (string, error) {
	if parts == nil {
		// If we didn't get an error previously
		return "", err
	} else if strings.HasPrefix(parts.MimeType, multipartPrefix) {
		var text, html, other string
		for _, part := range parts.Parts {
			switch part.MimeType {
			case "text/html":
				html = html2text.HTML2Text(part.Body)
			case "text/plain":
				text = part.Body
			default:
				other, err = getTextPart(&part, nil)
			}
		}
		if text != "" {
			return text, nil
		} else if html != "" {
			return html, nil
		} else {
			return other, err
		}
	} else if parts.MimeType == "text/plain" {
		return parts.Body, nil
	} else if parts.MimeType == "text/html" {
		return html2text.HTML2Text(parts.Body), nil
	}
	return "", InvalidError
}

func (s *Session) processBorrow(borrow string, m *mail.Message) error {
	parts, err := processMultipart(m.Header.Get(contentType), m.Header.Get(contentTransferEncoding), m.Body)
	body, err := getTextPart(parts, err)
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
	parts, err := processMultipart(m.Header.Get(contentType), m.Header.Get(contentTransferEncoding), m.Body)
	body, err := getTextPart(parts, err)
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
