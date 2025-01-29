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
	return DB{db}, nil
}

func (db *DB) Close() {
	db.Close()
}
