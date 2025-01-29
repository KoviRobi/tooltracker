package mail

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"regexp"
	"strings"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/k3a/html2text"
	"github.com/mcnijman/go-emailaddress"
	"github.com/mnako/letters"
)

// Data passed around during the processing of a single mail
type Session struct {
	Db   db.DB
	Dkim string
	From *string
}

var InvalidError = errors.New("Invalid email")

var verifyOptions = dkim.VerifyOptions{
	LookupTXT: net.LookupTXT,
}

var borrowRe = regexp.MustCompile(`^(?i)Borrowed[ +](.*)$`)

// Handle "Re:" and other localised versions
// TODO: Non-ASCII?
var aliasRe = regexp.MustCompile(`^(?i)(\w*:\s*)?Alias([ +].*)?\b`)

func (s *Session) Handle(buf []byte) error {
	reader := bytes.NewReader(buf)

	if s.From == nil {
		log.Println("No `from` in for this mail")
		return InvalidError
	}

	delegate := s.Db.GetDelegatedEmailFor(*s.From)

	err := s.verifyMail(delegate, reader)
	if err != nil {
		return err
	}

	reader.Seek(0, io.SeekStart)
	m, err := letters.ParseEmail(reader)
	if err != nil {
		log.Printf("Error parsing e-mail: %v", err)
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

func (s *Session) verifyMail(delegate string, reader *bytes.Reader) error {
	if s.Dkim != "" {
		dkimDomain := s.Dkim
		// At this point, we must have set an alias delegate using DKIM valid alias
		// command
		if *s.From != delegate {
			address, err := emailaddress.Parse(*s.From)
			if err != nil {
				log.Printf("Error parsing e-mail address %v", err)
				return InvalidError
			}
			dkimDomain = address.Domain
		}
		reader.Seek(0, io.SeekStart)
		verifications, err := dkim.VerifyWithOptions(reader, &verifyOptions)
		if err != nil {
			log.Printf("Error trying to verify e-mail: %v", err)
			return InvalidError
		}

		verified := false
		for _, verification := range verifications {
			if verification != nil {
				if verification.Err == nil {
					if verification.Domain == dkimDomain {
						verified = true
					} else {
						log.Printf("Verified %s but not the one we are looking for: %s\n",
							verification.Domain, dkimDomain)
					}
				} else {
					log.Printf("Failed to verify %s: %s\n", verification.Domain, verification.Err)
				}
			}
		}
		if !verified {
			log.Println("Failed to verify message")
			return InvalidError
		}
	}

	return nil
}

func (s *Session) processBorrow(body, borrow string) error {
	body = strings.TrimSpace(body)
	comment := strings.SplitN(body, "\n", 2)[0]
	comment = strings.TrimSpace(comment)
	location := db.Location{
		Tool:       borrow,
		LastSeenBy: *s.From,
		Comment:    &comment,
	}
	s.Db.UpdateLocation(location)

	return nil
}

func (s *Session) processAlias(body string, delegateFrom *string) error {
	body = strings.TrimSpace(body)
	alias := strings.SplitN(body, "\n", 2)[0]
	alias = strings.TrimSpace(alias)
	s.Db.UpdateAlias(db.Alias{
		Email: *s.From,
		Alias: alias,
	})

	if delegateFrom != nil {
		from := emailaddress.FindWithRFC5322([]byte(*delegateFrom), false)
		for _, address := range from {
			s.Db.UpdateAlias(db.Alias{
				Email:          address.String(),
				Alias:          alias,
				DelegatedEmail: s.From,
			})
		}
	}

	return nil
}
