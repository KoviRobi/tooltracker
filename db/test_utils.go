package db

import (
	"fmt"
	"testing"

	"github.com/KoviRobi/tooltracker/limits"
)

func CommonInit(t *testing.T) DB {
	limits.MaxMessageBytes = 1024
	limits.MaxRecipients = 1

	conn, err := Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	err = conn.EnsureTooltrackerTables()
	if err != nil {
		t.Fatal(err)
	}

	if conn.GetItems(nil) != nil {
		t.Fatalf("Expected DB to be empty at start")
	}

	return conn
}
