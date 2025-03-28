package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/imap"
	"github.com/KoviRobi/tooltracker/web"
)

// imapCmd represents the imap command
var imapCmd = &cobra.Command{
	Use:   "imap",
	Short: "Use IMAP to retreive mail",
	Long: `This mode works by using IMAP (with IDLE) to monitor a mailbox and act on
incoming mail.

Any mail, whether or not it was parsed successfully, is deleted (the assumption
is that other mail is spam, and doesn't need to be constantly parsed just to
fail).

So use a custom receiver, or at least a custom mailbox.`,
	Run: func(cmd *cobra.Command, args []string) {
		fromRe, err := regexp.Compile(from)
		if err != nil {
			log.Fatalf("Bad `from` regexp: %v", err)
		}

		dbConn, err := db.Open(dbPath)
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		defer dbConn.Close()

		err = dbConn.EnsureTooltrackerTables()
		if err != nil {
			log.Fatalf("Failed to ensure tooltracker tables exist: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		shutdownChan := make(chan struct{})
		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			<-c
			log.Printf("Got Ctrl-C, terminating")
			close(shutdownChan)
			wg.Wait()
		}()

		imapRetries := max(0, min(255, viper.GetInt("imap-retries")))
		errChan := make(chan error, imapRetries)

		httpServer := web.Server{
			Db:           dbConn,
			FromRe:       fromRe,
			To:           to,
			Domain:       domain,
			HttpPrefix:   httpPrefix,
			ErrorChan:    errChan,
			ShutdownChan: shutdownChan,
		}
		go func() {
			defer wg.Done()
			httpServer.Serve(fmt.Sprintf("%s:%d", listen, httpPort))
		}()

		imapSession := imap.Session{
			Db:           dbConn,
			Dkim:         dkim,
			Delegate:     delegate,
			LocalDkim:    localDkim,
			Host:         viper.GetString("imap-host"),
			User:         viper.GetString("imap-user"),
			Mailbox:      viper.GetString("mailbox"),
			TokenCmd:     viper.GetStringSlice("token-cmd"),
			IdlePoll:     viper.GetDuration("idle-poll"),
			ShutdownChan: shutdownChan,
		}

		go func() {
			defer wg.Done()
			for i := range imapRetries {
				err := imapSession.Listen()
				if err != nil {
					log.Printf("IMAP error %v", err)
					errChan <- err
				}
				select {
				case <-shutdownChan:
					return
				case <-time.After(
					// Exponetial backoff
					time.Duration(1<<i) * time.Second,
				):
					// Retry
				}
			}
		}()

		wg.Wait()
	},
}

func init() {
	rootCmd.AddCommand(imapCmd)
	imapCmd.Flags().String("imap-host", "outlook.office365.com:993",
		"host for IMAP to connect to")
	imapCmd.Flags().String("imap-user", "", "username to use for IMAP")
	imapCmd.Flags().String("mailbox", "INBOX", "mailbox to watch")
	imapCmd.Flags().StringArray("token-cmd",
		[]string{"pizauth", "show", "tooltracker"},
		"command to fetch authentication token (e.g. pizauth), specify multiple times for each argument")
	imapCmd.Flags().Duration("idle-poll", 2*time.Hour,
		"Time to reset IDLE connection in case it has crashed")
	imapCmd.Flags().Int("imap-retries", 3,
		"Number of times to retry IMAP connection, each time it waits twice as long as before, i.e. for the default of 3 it waits 1s, 2s, 4s")

	viper.BindPFlags(imapCmd.Flags())
}
