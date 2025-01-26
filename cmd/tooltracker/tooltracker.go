package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"

	"github.com/earthboundkid/versioninfo/v2"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/smtp"
	"github.com/KoviRobi/tooltracker/web"
)

var listen = flag.String("listen", "localhost", "host name/IP to listen on")
var domain = flag.String("domain", "localhost",
	"host name/IP to respond to HELO/EHLO, usually public FQDN or public IP."+
		" Also used for QR code")
var smtpPort = flag.Int("smtp", 1025, "port for SMTP to listen on")
var httpPort = flag.Int("http", 8123, "port for HTTP to listen on")
var httpPrefix = flag.String("http-prefix", "", "tooltracker HTTP prefix (default \"\", i.e. root)")
var from = flag.String("from", "^.*@work.com$",
	"regex for emails which are not anonimised")
var to = flag.String("to", "tooltracker", "name of mailbox to send mail to")
var dkim = flag.String("dkim", "", "name of domain to check for DKIM signature")
var dbPath = flag.String("db", db.FlagDbDefault, db.FlagDbDescription)

// ExampleServer runs an example SMTP server.
//
// It can be tested manually with e.g. netcat:
//
//	> unix2dos <<EOF | nc -N localhost 1025
//	EHLO localhost
//	MAIL FROM:<bob@user-mail.com>
//	RCPT TO:<tooltracker@instance.com>
//	DATA
//	Subject: Borrowed foo^M
//	^M
//	By my desk^M
//	.^M
//	QUIT
//	EOF
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	versioninfo.AddFlag(nil)
	flag.Parse()

	fromRe, err := regexp.Compile(*from)
	if err != nil {
		log.Fatalf("Bad `from` regexp: %v", err)
	}

	db, err := db.Open(*dbPath)
	defer db.Close()
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	err = db.EnsureTooltrackerTables()
	if err != nil {
		log.Fatalf("Failed to ensure tooltracker tables exist: %v", err)
	}

	httpServer := web.Server{
		Db:         db,
		FromRe:     fromRe,
		To:         *to,
		Domain:     *domain,
		HttpPrefix: *httpPrefix,
	}
	go httpServer.Serve(fmt.Sprintf("%s:%d", *listen, *httpPort))

	accept := fmt.Sprintf("%s@%s", *to, *domain)
	backend := smtp.Backend{
		Db:     db,
		To:     accept,
		Dkim:   *dkim,
		FromRe: fromRe,
	}

	smtpListen := fmt.Sprintf("%s:%d", *listen, *smtpPort)
	smtp.Serve(smtpListen, *domain, backend)
}
