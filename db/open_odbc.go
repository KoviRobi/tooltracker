//go:build odbc

package db

import (
	"database/sql"

	_ "github.com/alexbrainman/odbc"
)

const (
	FlagDbDefault     = "Driver=SQLite;Database=tooltracker.db"
	FlagDbDescription = "ODBC connection path"
)

func Open(path string) (DB, error) {
	db, err := sql.Open("odbc", path)
	if err != nil {
		return DB{}, err
	}

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS tracker (tool TEXT PRIMARY KEY, lastSeenBy TEXT NOT NULL, comment TEXT);
	CREATE TABLE IF NOT EXISTS tool (name TEXT PRIMARY KEY, description text, image TEXT);
	CREATE TABLE IF NOT EXISTS aliases (email TEXT PRIMARY KEY, alias TEXT NOT NULL, delegatedEmail TEXT);
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		return DB{}, err
	}
	return DB{db}, nil
}
