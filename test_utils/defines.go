package test_utils

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/limits"
)

const (
	To      = "tooltracker@a.example.com"
	Domain1 = "a.example.com"
	Domain2 = "b.example.com"
	Domain3 = "c.example.com"
	Tool1   = "tool1"
	Tool2   = "tool2"
)

var (
	User1  = fmt.Sprintf("user1@%s", Domain1)
	User2  = fmt.Sprintf("user2@%s", Domain1)
	User3  = fmt.Sprintf("user3@%s", Domain2)
	User4  = fmt.Sprintf("user4@%s", Domain2)
	User5  = fmt.Sprintf("user5@%s", Domain3)
	FromRe = regexp.MustCompile(fmt.Sprintf(".*@%s", regexp.QuoteMeta(Domain1)))
)

const (
	Borrow        = "Borrowed "
	Alias         = "Alias "
	PlainTemplate = `From: %s
To: %s
Subject: %s

%s
`
)

const TestPrivateKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQCzhPzglJHjTp6Yf+slxL4wyh+Xmdh478MjKQpStLEB2EML+5oJ
N+5x/cKa6TxChqZGVwwJrUyKMXUmwO8GI74FNWtCCAs0oeV2TtUVchTJmwWolAX0
WqIc3A60pZiXtcmUfmtW/pZz6bKsTua4zY+SeHvdjsBECOuJ6RNQRJPj9wIDAQAB
AoGAa6U5BWnROR4xl3xNAq7A5PyuiPdliM8ske7QE9vpsBN/0LWkHhb90ji58q4c
xj97gP49Z6gVF2CkwQI70dCo5fDx7YBKb8qQU/DlPODK2KPny5rwaYkzxaN5rCdc
tQwTUQvxdNqKwmLe8Za/TQ9TT2+HUFhv8G5zn/UjINMg8PECQQDl75HpyudgCy7q
QP1TjQ78XGmZTSEBC8l6ZP0RslYBt/hkqOY6bwNCHbVO+vo5Zd/NF7cV2govO6EC
NcE0jqTDAkEAx95nNVn0ZTLYKMhGg5u3+FWhnVHHZGR4/LIEzgpr6T/XVW1vtf6T
fuJ+z/g6JsWbWxPlBJc27euXY7zvcmbAvQJAYGqx28A6f1qRJKd10oguxYGWwjLG
aSLhLFKWj8ohKH1VShhM2incyuecNG8nZ9QhIWYVXrNcW+v8GuohhwFdcwJADRn/
GfgzlQ6oLMQ0GxxyCs1SMsXRlDsh0y64Mels+XU94FO0JvHxKTgfp/JVnYUGkgnT
0WE4MJBo9BjGeXFS4QJBAJNVgYLMGwl07BRJSqkVLggq2Bwgjs04QjXXeaIE4/Wc
OjYm8GPa+wBvAsXqv47hcLfYgo6cyANVJW4L8fzBOGo=
-----END RSA PRIVATE KEY-----
`

func CommonInit(t *testing.T) db.DB {
	limits.MaxMessageBytes = 1024
	limits.MaxRecipients = 1

	conn, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
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
