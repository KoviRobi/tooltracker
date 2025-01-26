package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/KoviRobi/tooltracker/db"
)

var listen, domain, httpPrefix, from, to, dkim, dbPath string
var httpPort int

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tooltracker",
	Short: "A web server + email client to track things with QR codes",
	Long: `This tracker works by printing QR codes with the following link:
"mailto:rcpt@org.com?subject=Borrowed%20<tool>" which when scanned, should open
up the email application on most mobile phones.

This way there is nothing to install on the users' phones. On the mail side, it
supports being an SMTP server to receive mail, or IMAP to download mail from a
mailbox.

It also acts as a web server, to display who has last seen which tool.`,
}

func init() {
	AddVersionFlag(rootCmd.PersistentFlags())

	rootCmd.PersistentFlags().StringVar(&listen, "listen", "localhost", "host name/IP to listen on")
	rootCmd.PersistentFlags().StringVar(&domain, "domain", "localhost",
		"host name/IP to respond to HELO/EHLO, usually public FQDN or public IP."+
			" Also used for QR code")
	rootCmd.PersistentFlags().IntVar(&httpPort, "http", 8123, "port for HTTP to listen on")
	rootCmd.PersistentFlags().StringVar(&httpPrefix, "http-prefix", "", "tooltracker HTTP prefix (default \"\", i.e. root)")
	rootCmd.PersistentFlags().StringVar(&from, "from", "^.*@work.com$",
		"regex for emails which are not anonimised")
	rootCmd.PersistentFlags().StringVar(&to, "to", "tooltracker", "name of mailbox to send mail to")
	rootCmd.PersistentFlags().StringVar(&dkim, "dkim", "", "name of domain to check for DKIM signature")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", db.FlagDbDefault, db.FlagDbDescription)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
