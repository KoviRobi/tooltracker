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
	Db        db.DB
	From      *string
	Dkim      string
	Delegate  bool
	LocalDkim bool
}

var ErrInvalid = errors.New("Invalid email")

var verifyOptions = dkim.VerifyOptions{
	LookupTXT: net.LookupTXT,
}

// Treat a double newline as a signature delimiter
var signatureRe = regexp.MustCompile(`(\p{Zs}*\r?\n){2}`)
// Fix up divs to contain an extra new-line -- simple work-around to signatures
// being separated by `</div>` and `<br>`
var htmlNewlineTags = regexp.MustCompile(`</\s*div>`)

var borrowRe = regexp.MustCompile(`^(?i)Borrowed[ +](.*)$`)

// Handle "Re:" and other localised versions
// TODO: Non-ASCII?
var aliasRe = regexp.MustCompile(`^(?i)(\w*:\s*)?Alias([ +].*)?\b`)

func (s *Session) Handle(buf []byte) error {
	reader := bytes.NewReader(buf)

	if s.From == nil {
		log.Println("No `from` in for this mail")
		return ErrInvalid
	}

	// Delegation example: Assuming Dkim is work.com but bob@work.com has sent
	// "Alias bob@family.net", then delegate of bob@family.net is
	// bob@work.com (if delegation is enabled, otherwise it is unchanged)
	delegate := *s.From
	if s.Delegate {
		delegate = s.Db.GetDelegatedEmailFor(*s.From)
	}

	err := s.verifyMail(delegate, reader)
	if err != nil {
		return err
	}

	reader.Seek(0, io.SeekStart)
	m, err := letters.ParseEmail(reader)
	if err != nil {
		log.Printf("Error parsing e-mail: %v", err)
		return ErrInvalid
	}

	subject := m.Headers.Subject
	body := m.Text
	if body == "" {
		m.HTML = htmlNewlineTags.ReplaceAllString(m.HTML, `$1<br>`)
		body = html2text.HTML2Text(m.HTML)
	}
	signatureStart := signatureRe.FindStringIndex(body)
	if signatureStart != nil {
		body = body[:signatureStart[0]]
	}
	body = strings.TrimSpace(body)
	log.Printf("Mail body: %q", body[:min(len(body), 100)])
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
		return ErrInvalid
	}
}

func (s *Session) verifyMail(delegate string, reader *bytes.Reader) error {
	if s.Dkim != "" {
		dkimDomain := s.Dkim
		address, err := emailaddress.Parse(*s.From)
		if err != nil {
			log.Printf("Error parsing e-mail address %v", err)
			return ErrInvalid
		}
		// At this point, we must have set an alias delegate using DKIM valid alias
		// command
		if *s.From != delegate {
			dkimDomain = address.Domain
		} else if /* *s.From == delegate && */ address.Domain == s.Dkim && !s.LocalDkim {
			return nil
		}

		reader.Seek(0, io.SeekStart)
		verifications, err := dkim.VerifyWithOptions(reader, &verifyOptions)
		if err != nil {
			log.Printf("Error trying to verify e-mail: %v", err)
			return ErrInvalid
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
			return ErrInvalid
		}
	}

	return nil
}

func (s *Session) processBorrow(body, borrow string) error {
	location := db.Location{
		Tool:       borrow,
		LastSeenBy: *s.From,
		Comment:    &body,
	}
	s.Db.UpdateLocation(location)

	return nil
}

func (s *Session) processAlias(body string, delegateFrom *string) error {
	s.Db.UpdateAlias(db.Alias{
		Email: *s.From,
		Alias: body,
	})

	if delegateFrom != nil {
		from := emailaddress.FindWithRFC5322([]byte(*delegateFrom), false)
		for _, address := range from {
			s.Db.UpdateAlias(db.Alias{
				Email:          address.String(),
				Alias:          body,
				DelegatedEmail: s.From,
			})
		}
	}

	return nil
}
