// This module listens on an IMAP connection, initially reading all mail in the
// given folder, then starting an IDLE connection, and reading new mail.
// Whenever it has processed an email (successfully or failed due to some
// parsing error), it deletes it.
package imap

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/limits"
	"github.com/KoviRobi/tooltracker/mail"
)

type Session struct {
	Db        db.DB
	Dkim      string
	Host      string
	User      string
	Mailbox   string
	TokenCmd  []string
	Delegate  bool
	LocalDkim bool
}

func (s *Session) Listen() error {
	shutdownChan := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(1)
	defer wg.Done()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Printf("Got Ctrl-C, terminating")
		close(shutdownChan)
		wg.Wait()
	}()

	token, err := s.getToken()
	if err != nil {
		log.Printf("failed to get token: %v", err)
		return err
	}

	var c *imapclient.Client
	idleReceived := make(chan uint32, 1)
	var idleCmd *imapclient.IdleCommand

	options := imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					idleReceived <- *data.NumMessages
				}
			},
		},
	}

	host := s.Host
	c, err = imapclient.DialTLS(host, &options)
	if err != nil {
		log.Printf("failed to dial IMAP server: %v", err)
		return err
	}
	defer c.Close()

	var saslClient sasl.Client
	if c.Caps().Has(imap.AuthCap(sasl.OAuthBearer)) {
		log.Println("Using OAUTHBEARER")
		saslClient = sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
			Username: s.User,
			Token:    token,
		})
	} else if c.Caps().Has(imap.AuthCap("XOAUTH2")) {
		log.Println("Using XOAUTH2")
		saslClient = NewXoauth2Client(
			s.User,
			token,
		)
	}
	if err := c.Authenticate(saslClient); err != nil {
		log.Printf("Authentication failed: %v", err)
		return err
	}

	selectedMbox, err := c.Select(s.Mailbox, nil).Wait()
	if err != nil {
		log.Printf("Failed to select %s: %v", s.Mailbox, err)
		return err
	}
	log.Printf("Mailbox %s contains %v messages", s.Mailbox, selectedMbox.NumMessages)
	numMessages := selectedMbox.NumMessages

idleLoop:
	for {
		if numMessages > 0 {
			s.fetchMessages(numMessages, c, shutdownChan)
		}

		select {
		case <-shutdownChan:
			log.Printf("Shutting down")
			break idleLoop
		default:
		}

		log.Printf("Entering IDLE")

		idleCmd, err = c.Idle()
		if err != nil {
			log.Printf("IDLE command failed: %v", err)
			return err
		}

		select {
		case <-shutdownChan:
			log.Printf("Shutting down")
			break idleLoop
		case n := <-idleReceived:
			log.Printf("IDLE got %d messages", n)
			numMessages = n
		}

		// Stop idling -- to fetch another message
		if err := idleCmd.Close(); err != nil {
			log.Printf("Failed to stop idling: %v", err)
			return err
		}
	}

	// Stop idling -- we are shutting down
	if err := idleCmd.Close(); err != nil {
		log.Printf("Failed to stop idling: %v", err)
		return err
	}

	select {
	case <-shutdownChan:
		return errors.New("Ctrl-C")
	default:
		return nil
	}
}

// Fetch from IMAP and forward messages to the tooltracker mail handler
func (s *Session) fetchMessages(
	numMessages uint32,
	c *imapclient.Client,
	shutdownChan chan struct{},
) {
	var next uint32 = 0
	for next < numMessages {
		select {
		case <-shutdownChan:
			log.Printf("Shutting down")
			return
		default:
		}

		// IMAP uses 1-based numbering so increment here
		next = next + 1
		log.Printf("Fetching seq-num %d", next)
		seqSet := imap.SeqSetNum(next)
		bodySection := &imap.FetchItemBodySection{
			Partial: &imap.SectionPartial{Offset: 0, Size: int64(limits.MaxMessageBytes)},
		}
		fetchOptions := &imap.FetchOptions{
			Envelope:    true,
			BodySection: []*imap.FetchItemBodySection{bodySection},
		}
		messages, err := c.Fetch(seqSet, fetchOptions).Collect()

		if err != nil {
			log.Printf("Failed to fetch message %d in %s: %v", next, s.Mailbox, err)
		} else if len(messages) == 0 {
			log.Printf("Fetched zero messages")
		} else {
			for _, message := range messages {
				s.forwardMessage(message, c)
			}
		}
	}
	_, err := c.Expunge().Collect()
	if err != nil {
		log.Printf("Failed to expunge messages: %v", err)
	}
}

// Forward a message to the tooltracker mail handler
func (s *Session) forwardMessage(
	message *imapclient.FetchMessageBuffer,
	c *imapclient.Client,
) {
	if len(message.Envelope.From) != 1 {
		log.Printf("Expecting one from address, got %d", len(message.Envelope.From))
	} else if len(message.BodySection) != 1 {
		log.Printf("Expecting one body but got %d", len(message.BodySection))
	} else {
		from := message.Envelope.From[0].Addr()
		var body []byte
		for _, body = range message.BodySection {
			break
		}
		session := mail.Session{
			Db:        s.Db,
			Dkim:      s.Dkim,
			Delegate:  s.Delegate,
			LocalDkim: s.LocalDkim,
			From:      &from,
		}
		log.Printf("Processing message from %s subject %s", from, message.Envelope.Subject)
		session.Handle(body)
	}
	storeFlags := imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagDeleted},
	}
	msgs, err := c.Store(imap.SeqSetNum(message.SeqNum), &storeFlags, nil).Collect()
	if err != nil {
		log.Printf("Got error setting deleted on message: %v", err)
	} else if len(msgs) != 1 {
		log.Printf(
			"Expected to delete 1 message, deleted %d -- is someone also operating on this mailbox?",
			len(msgs),
		)
		for i, msg := range msgs {
			decoration := "Unexpected"
			if msg == nil {
				log.Printf("Message %d is <nil> -- bug in imapclient?", i)
			} else {
				if msg.SeqNum == message.SeqNum {
					decoration = "Expected"
				}
				log.Printf("%s message subject: %s", decoration, msg.Envelope.Subject)
			}
		}
	}
}

func (s *Session) getToken() (token string, err error) {
	if len(s.TokenCmd) < 1 {
		err = errors.New("Token command invalid")
		return
	}

	tokenCmd := exec.Command(s.TokenCmd[0], s.TokenCmd[1:]...)
	tokenOut, err := tokenCmd.StdoutPipe()
	if err != nil {
		return
	}

	err = tokenCmd.Start()
	if err != nil {
		return
	}

	tokenBuf := make([]byte, 4096)
	n, err := tokenOut.Read(tokenBuf)
	if n == 0 && err != nil {
		return
	}

	err = tokenCmd.Wait()
	if err != nil {
		return
	}

	token = string(tokenBuf[:n])
	return
}
