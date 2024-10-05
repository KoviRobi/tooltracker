package main

import (
	"log"

	"github.com/KoviRobi/tooltracker/smtp"
)

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

	smtp.Serve()
}
