package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
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
	Image       string
}

type Item struct {
	Location
	Description *string
	Alias       *string
}

func (l Location) String() string {
	comment := "<nil>"
	if l.Comment != nil {
		comment = fmt.Sprintf("%q", *l.Comment)
	}
	return fmt.Sprintf("Location{\n\tTool: %q\n\tLastSeenBy: %q\n\tComment: %s\n}\n",
		l.Tool, l.LastSeenBy, comment)
}

func (a Alias) String() string {
	delegatedEmail := "<nil>"
	if a.DelegatedEmail != nil {
		delegatedEmail = fmt.Sprintf("%q", *a.DelegatedEmail)
	}
	return fmt.Sprintf("Alias{\n\tEmail: %q\n\tAlias: %q\n\tDelegatedEmail: %s\n}\n",
		a.Email, a.Alias, delegatedEmail)
}

func (t Tool) String() string {
	description := "<nil>"
	if t.Description != nil {
		description = fmt.Sprintf("%q", *t.Description)
	}
	return fmt.Sprintf("Tool{\n\tName: %q\n\tDescription: %s\n\tImage: %.10v\n}\n",
		t.Name, description, t.Image)
}

func (i Item) String() string {
	location := strings.ReplaceAll(i.Location.String(), "\n", "\n\t")
	description := "<nil>"
	if i.Description != nil {
		description = fmt.Sprintf("%q", *i.Description)
	}
	alias := "<nil>"
	if i.Alias != nil {
		alias = fmt.Sprintf("%q", *i.Alias)
	}
	return fmt.Sprintf("Item{\n\tLocation: %sAlias: %s\n\tDelegatedEmail: %s\n}\n",
		location, description, alias)
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

	_, err = stmt.Exec(
		strings.TrimSpace(location.Tool),
		strings.TrimSpace(location.LastSeenBy),
		NormalizeStringP(location.Comment),
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
		`SELECT name, description, image FROM tool WHERE name = ?`)
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
		LEFT JOIN tool ON tool.name = tracker.tool
		LEFT JOIN aliases ON aliases.email = tracker.lastSeenBy`)
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
		`SELECT delegatedEmail FROM aliases WHERE email = ?`)
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
