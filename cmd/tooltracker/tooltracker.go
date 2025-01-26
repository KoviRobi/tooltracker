package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/KoviRobi/tooltracker/db"
)

var cfgFile, listen, domain, httpPrefix, from, to, dkim, dbPath string
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
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("listen", "localhost", "host name/IP to listen on")
	rootCmd.PersistentFlags().String("domain", "localhost",
		"host name/IP to respond to HELO/EHLO, usually public FQDN or public IP."+
			" Also used for QR code")
	rootCmd.PersistentFlags().Int("http", 8123, "port for HTTP to listen on")
	rootCmd.PersistentFlags().String("http-prefix", "", "tooltracker HTTP prefix (default \"\", i.e. root)")
	rootCmd.PersistentFlags().String("from", "^.*@work.com$",
		"regex for emails which are not anonimised")
	rootCmd.PersistentFlags().String("to", "tooltracker", "name of mailbox to send mail to")
	rootCmd.PersistentFlags().String("dkim", "", "name of domain to check for DKIM signature")
	rootCmd.PersistentFlags().String("db", db.FlagDbDefault, db.FlagDbDescription)

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
	domain = viper.GetString("domain")
	from = viper.GetString("from")
	httpPort = viper.GetInt("http")
	httpPrefix = viper.GetString("http-prefix")
	listen = viper.GetString("listen")
	to = viper.GetString("to")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
