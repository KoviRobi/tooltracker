package main

import (
	"fmt"
	"log"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/smtp"
	"github.com/KoviRobi/tooltracker/web"
)

// smtpCmd represents the smtp command
var smtpCmd = &cobra.Command{
	Use:   "smtp",
	Short: "Use SMTP to receive mail",
	Long: `This mode works by listening on a port (e.g. 25) using the SMTP protocol for
new mail. That is, it acts as a message transfer agent (MTA).

If you can't set up local forwarding from your org's MTA for the specific
tooltracker email address to this server, using only local ports (e.g. because
your mail is done by Microsoft/Google/etc), then consider using the IMAP
mode.

If you want to try out tooltracker locally, SMTP (with e.g. a user port, i.e.
port >= 1024) can be used alongside with a tool such as netcat:

	> unix2dos <<EOF | nc -N localhost 1025
	EHLO localhost
	MAIL FROM:<bob@user-mail.com>
	RCPT TO:<tooltracker@instance.com>
	DATA
	Subject: Borrowed foo^M
	^M
	By my desk^M
	.^M
	QUIT
	EOF`,
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

		accept := fmt.Sprintf("%s@%s", to, domain)
		backend := smtp.Backend{
			Db:        dbConn,
			To:        accept,
			Dkim:      dkim,
			Delegate:  delegate,
			LocalDkim: localDkim,
			FromRe:    fromRe,
		}

		smtpListen := fmt.Sprintf("%s:%d", listen, viper.GetInt("smtp-port"))
		smtp.Serve(smtpListen, domain, backend)
	},
}

func init() {
	rootCmd.AddCommand(smtpCmd)
	smtpCmd.Flags().Int("smtp-port", 1025, "port for SMTP to listen on")

	viper.BindPFlags(rootCmd.Flags())
}
