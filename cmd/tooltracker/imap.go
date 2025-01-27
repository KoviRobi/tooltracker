package main

import (
	"fmt"
	"log"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/imap"
	"github.com/KoviRobi/tooltracker/web"
)

var imapHost, imapUser, mailbox string
var tokenCmd []string

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

		httpServer := web.Server{
			Db:         dbConn,
			FromRe:     fromRe,
			To:         to,
			Domain:     domain,
			HttpPrefix: httpPrefix,
		}
		go httpServer.Serve(fmt.Sprintf("%s:%d", listen, httpPort))

		imapSession := imap.Session{
			Db:        dbConn,
			Dkim:      dkim,
			Delegate:  delegate,
			LocalDkim: localDkim,
			Host:      viper.GetString("imap-host"),
			User:      viper.GetString("imap-user"),
			Mailbox:   viper.GetString("mailbox"),
			TokenCmd:  viper.GetStringSlice("token-cmd"),
		}

		err = imapSession.Listen()
		if err != nil {
			log.Fatalf("IMAP error %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(imapCmd)
	imapCmd.Flags().StringVar(&imapHost, "imap-host", "outlook.office365.com:993",
		"host for IMAP to connect to")
	imapCmd.Flags().StringVar(&imapUser, "imap-user", "", "username to use for IMAP")
	imapCmd.Flags().StringVar(&mailbox, "mailbox", "INBOX", "mailbox to watch")
	imapCmd.Flags().StringArrayVar(&tokenCmd, "token-cmd",
		[]string{"pizauth", "show", "tooltracker"},
		"command to fetch authentication token (e.g. pizauth), specify multiple times for each argument")

	viper.BindPFlags(imapCmd.Flags())
}
