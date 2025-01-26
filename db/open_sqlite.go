//go:build !odbc

package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

const (
	FlagDbDefault     = "tooltracker.db"
	FlagDbDescription = "path to sqlite3 file to create/use"
)

func Open(path string) (DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return DB{}, err
	}
	return DB{db}, nil
}

func (db *DB) Close() {
	db.DB.Close()
}
