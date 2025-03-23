package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/limits"
)

var (
	cfgFile, listen, domain, httpPrefix, from, to, dkim, dbPath string
	localDkim, delegate                                         bool
	httpPort                                                    int
)

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
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("listen", "localhost", "host name/IP to listen on")
	rootCmd.PersistentFlags().String("domain", "localhost",
		"Domain part (the @...) of the e-mail."+
			" Used to respond to HELO/EHLO, usually public FQDN or public IP."+
			" Also used for QR code.")
	rootCmd.PersistentFlags().Int("http-port", 8123, "port for HTTP to listen on")
	rootCmd.PersistentFlags().String("http-prefix", "", "tooltracker HTTP prefix (default \"\", i.e. root)")
	rootCmd.PersistentFlags().String("from", "^.*@work.com$",
		"regex for emails which are not anonimised")
	rootCmd.PersistentFlags().String("to", "tooltracker", "local part of the e-mail to send mail to (the ...@)")
	rootCmd.PersistentFlags().String("dkim", "",
		`name of domain to check for DKIM signature (otherwise domains aren't
checked because they are trivially forged`)
	rootCmd.PersistentFlags().Bool("delegate", true, "e-mail delegation, when using DKIM")
	rootCmd.PersistentFlags().Bool("local-dkim", true,
		"e-mails from the same domain as tooltracker is running on don't get DKIM")
	rootCmd.PersistentFlags().String("db", db.FlagDbDefault, db.FlagDbDescription)

	rootCmd.PersistentFlags().Uint32("max-message-bytes", 1024*1024, "Maximum bytes to process per e-mail (to prevent DoS)")
	rootCmd.PersistentFlags().Uint32("max-recipients", 10, "Maximum recipients to process per e-mail (to prevent DoS)")
	rootCmd.PersistentFlags().Duration("read-timeout", 10*time.Second, "Read timeout for servers")
	rootCmd.PersistentFlags().Duration("write-timeout", 10*time.Second, "Write timeout for servers")

	viper.BindPFlags(rootCmd.PersistentFlags())

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /etc/tooltracker.yaml)")
	AddVersionFlag(rootCmd.PersistentFlags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search config in /etc with name "tooltracker" (without extension).
		viper.AddConfigPath("/etc")
		viper.SetConfigName("tooltracker")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}

	dbPath = viper.GetString("db")
	dkim = viper.GetString("dkim")
	delegate = viper.GetBool("delegate")
	localDkim = viper.GetBool("local-dkim")
	domain = viper.GetString("domain")
	from = viper.GetString("from")
	httpPort = viper.GetInt("http-port")
	httpPrefix = viper.GetString("http-prefix")
	listen = viper.GetString("listen")
	to = viper.GetString("to")

	limits.MaxMessageBytes = viper.GetUint32("max-message-bytes")
	limits.MaxRecipients = viper.GetUint32("max-recipients")
	limits.ReadTimeout = viper.GetDuration("read-timeout")
	limits.WriteTimeout = viper.GetDuration("write-timeout")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
