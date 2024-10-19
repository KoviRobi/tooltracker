package db

import (
	"database/sql"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct{ *sql.DB }

type Location struct {
	Tool       string
	LastSeenBy string
	Comment    *string
}

type Alias struct {
	Email          string
	Alias          string
	DelegatedEmail *string
}

type Tool struct {
	Name        string
	Description *string
	Image       []byte
}

type Item struct {
	Location
	Description *string
	Alias       *string
}

func Open(path string) (DB, error) {
	_, err := os.Stat(path)
	create := os.IsNotExist(err)

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return DB{}, err
	}

	if create {
		sqlStmt := `
		CREATE TABLE tracker (tool text primary key, lastSeenBy text NOT NULL, comment text);
		CREATE TABLE tool (name text primary key, description text, image blob);
		CREATE TABLE aliases (email text primary key, alias text NOT NULL, delegatedEmail text);
		`
		_, err = db.Exec(sqlStmt)
		if err != nil {
			return DB{}, err
		}
	}
	return DB{db}, nil
}

// Represent "" as nil, and trim spaces.
func NormalizeStringP(s *string) *string {
	if s == nil {
		return s
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (db DB) UpdateLocation(location Location) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	stmt, err := tx.Prepare(`
	INSERT INTO tracker (tool, lastSeenBy, comment) VALUES (?, ?, ?)
		ON CONFLICT(tool) DO UPDATE SET
			lastSeenBy=excluded.lastSeenBy,
			comment=excluded.comment`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	var comment string
	if location.Comment != nil {
		comment = strings.TrimSpace(*location.Comment)
	}

	_, err = stmt.Exec(
		strings.TrimSpace(location.Tool),
		strings.TrimSpace(location.LastSeenBy),
		comment,
	)
	if err != nil {
		log.Fatal(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}
}

func (db DB) UpdateTool(tool Tool) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := tx.Prepare(`
	INSERT INTO tool (name, description, image) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			description=excluded.description,
			image=excluded.image`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		strings.TrimSpace(tool.Name),
		NormalizeStringP(tool.Description),
		tool.Image,
	)
	if err != nil {
		log.Fatal(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}
}

func (db DB) UpdateAlias(alias Alias) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := tx.Prepare(`
	INSERT INTO aliases (email, alias, delegatedEmail) VALUES (?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			alias=excluded.alias,
			delegatedEmail=coalesce(excluded.delegatedEmail, delegatedEmail)`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		strings.TrimSpace(alias.Email),
		strings.TrimSpace(alias.Alias),
		NormalizeStringP(alias.DelegatedEmail))
	if err != nil {
		log.Fatal(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}
}

func (db DB) GetTool(name string) (tool Tool) {
	stmt, err := db.Prepare(
		`SELECT name, description, image FROM tool WHERE name == ?`)
	if err != nil {
		log.Fatal(err)
	}

	err = stmt.QueryRow(name).Scan(&tool.Name, &tool.Description, &tool.Image)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal(err)
	}

	return
}

func (db DB) GetItems() []Item {
	var items []Item

	rows, err := db.Query(`
	SELECT tracker.tool, tool.description, tracker.lastSeenBy, aliases.alias, tracker.comment
		FROM tracker
		LEFT JOIN tool ON tool.name == tracker.tool
		LEFT JOIN aliases ON aliases.email == tracker.lastSeenBy`)
	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var item Item
		err = rows.Scan(&item.Tool, &item.Description, &item.LastSeenBy, &item.Alias, &item.Comment)
		if err != nil {
			log.Fatal(err)
		}
		items = append(items, item)
	}

	return items
}

func (db DB) GetDelegatedEmailFor(from string) string {
	var delegate sql.NullString
	stmt, err := db.Prepare(
		`SELECT delegatedEmail FROM aliases WHERE email == ?`)
	if err != nil {
		log.Fatal(err)
	}

	err = stmt.QueryRow(from).Scan(&delegate)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal(err)
	}
	if delegate.Valid {
		return delegate.String
	} else {
		return from
	}
}
