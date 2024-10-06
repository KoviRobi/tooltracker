package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/smtp"
	"github.com/KoviRobi/tooltracker/web"
)

var listen = flag.String("listen", "localhost", "host name/IP to listen on")
var domain = flag.String("domain", "localhost",
	"host name/IP to respond to HELO/EHLO, usually public FQDN or public IP")
var smtpPort = flag.Int("smtp", 1025, "port for SMTP to listen on")
var httpPort = flag.Int("http", 8123, "port for HTTP to listen on")
var from = flag.String("from", "^.*@work.com$",
	"regex for emails which are not anonimised")
var to = flag.String("to", "tooltracker", "name of mailbox to send mail to")
var dbPath = flag.String("db", "tooltracker.db", "path to sqlite3 file to create/use")

// ExampleServer runs an example SMTP server.
//
// It can be tested manually with e.g. netcat:
//
//	> netcat -C localhost 1025
//	EHLO localhost
//	MAIL FROM:<bob@user-mail.com>
//	RCPT TO:<tooltracker@instance.com>
//	DATA
//	Subject: Borrowed foo^M
//	^M
//	By my desk^M
//	.^M
//	QUIT
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()

	fromRe, err := regexp.Compile(*from)
	if err != nil {
		log.Fatal(err)
	}

	db, err := db.Open("test.db")
	if err != nil {
		log.Fatal(err)
	}

	go web.Serve(db, fmt.Sprintf("%s:%d", *listen, *httpPort), fromRe)

	accept := fmt.Sprintf("%s@%s", *to, *domain)
	smtp.Serve(db, fmt.Sprintf("%s:%d", *listen, *smtpPort), *domain, accept)
}
