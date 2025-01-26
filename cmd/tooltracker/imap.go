package main

import (
	"fmt"

	"github.com/spf13/cobra"
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
		fmt.Println("imap called")
	},
}

func init() {
	rootCmd.AddCommand(imapCmd)
}
