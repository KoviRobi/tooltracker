package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	tagsModule "github.com/KoviRobi/tooltracker/tags"
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
	Tags        []string
	Description *string
	Image       string
}

type Item struct {
	Location
	Tags        *[]string
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

func (db DB) EnsureTooltrackerTables() error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS tracker (tool TEXT PRIMARY KEY, lastSeenBy TEXT NOT NULL, comment TEXT);
	CREATE TABLE IF NOT EXISTS tool (name TEXT PRIMARY KEY, description text, image TEXT);
	CREATE TABLE IF NOT EXISTS aliases (email TEXT PRIMARY KEY, alias TEXT NOT NULL, delegatedEmail TEXT);
	CREATE TABLE IF NOT EXISTS tags (tag TEXT, tool TEXT, PRIMARY KEY (tag, tool));
	`
	_, err := db.Exec(sqlStmt)
	return err
}

func (db DB) UpdateLocation(location Location) {
	stmt, err := db.Prepare(`
	INSERT INTO tracker (tool, lastSeenBy, comment) VALUES (?, ?, ?)
		ON CONFLICT(tool) DO UPDATE SET
			lastSeenBy=excluded.lastSeenBy,
			comment=excluded.comment`)
	if err != nil {
		log.Printf("Error preparing query: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		strings.TrimSpace(location.Tool),
		strings.TrimSpace(location.LastSeenBy),
		NormalizeStringP(location.Comment),
	)
	if err != nil {
		log.Fatal("Error executing database query: $v", err)
	}
}

func (db DB) UpdateTool(tool Tool) {
	stmt, err := db.Prepare(`
	INSERT INTO tool (name, description, image) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			description=excluded.description,
			image=excluded.image`)
	if err != nil {
		log.Printf("Error preparing query: %v", err)
	}
	defer stmt.Close()

	name := strings.TrimSpace(tool.Name)
	_, err = stmt.Exec(
		name,
		NormalizeStringP(tool.Description),
		tool.Image,
	)
	if err != nil {
		log.Printf("Error executing query: %v", err)
	}

	db.UpdateTags(name, tool.Tags)
}

func (db DB) UpdateTags(tool string, tags []string) {
	_, err := db.Exec(`DELETE FROM tags WHERE tags.tool = ?`, tool)
	if err != nil {
		log.Printf("Error dropping previous tags: %v", err)
	}
	stmt, err := db.Prepare(`
	INSERT INTO tags (tag, tool) VALUES (?, ?)`)
	if err != nil {
		log.Printf("Error preparing query: %v", err)
	}
	defer stmt.Close()

	for _, tag := range tags {
		_, err = stmt.Exec(tag, tool)
		if err != nil {
			log.Printf("Error executing query: %v", err)
		}
	}
}

func (db DB) UpdateAlias(alias Alias) {
	stmt, err := db.Prepare(`
	INSERT INTO aliases (email, alias, delegatedEmail) VALUES (?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			alias=excluded.alias,
			delegatedEmail=coalesce(excluded.delegatedEmail, delegatedEmail)`)
	if err != nil {
		log.Printf("Error preparing query: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		strings.TrimSpace(alias.Email),
		strings.TrimSpace(alias.Alias),
		NormalizeStringP(alias.DelegatedEmail))
	if err != nil {
		log.Printf("Error executing query: %v", err)
	}
}

func (db DB) GetTool(name string) (tool Tool) {
	stmt, err := db.Prepare(`
		SELECT tool.name, string_agg(tags.tag, " "), tool.description, tool.image
		FROM tool
		LEFT JOIN tags ON tool.name = tags.tool
		WHERE tool.name = ?
		GROUP BY tool.name
		`)
	if err != nil {
		log.Printf("Error preparing query: %v", err)
	}
	defer stmt.Close()

	var itemTags *string
	err = stmt.QueryRow(name).Scan(&tool.Name, &itemTags, &tool.Description, &tool.Image)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Error getting rows from query: %v", err)
	}
	if itemTags != nil {
		tool.Tags = strings.Split(*itemTags, " ")
	}
	return
}

func (db DB) GetItems(tags []string) []Item {
	var items []Item
	var args []any
	tagFilter := ``

	if tags != nil {
		var filter string
		filter, args = tagsModule.TagsSqlFilter(tags)
		if filter != `` {
			tagFilter += `  WHERE ` + filter
		}
	}
	query := `
	SELECT tracker.tool, string_agg(tags.tag, " "), tool.description, tracker.lastSeenBy, aliases.alias, tracker.comment
		FROM tracker
		LEFT JOIN tags ON tracker.tool = tags.tool
		LEFT JOIN tool ON tool.name = tracker.tool
		LEFT JOIN aliases ON aliases.email = tracker.lastSeenBy
		` + tagFilter + `
		GROUP BY tracker.tool`
	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error executing query: %v", err)
	}

	var itemTags *string
	for rows.Next() {
		var item Item
		err = rows.Scan(&item.Tool, &itemTags, &item.Description, &item.LastSeenBy, &item.Alias, &item.Comment)
		if err != nil {
			log.Printf("Error getting row from query: %v", err)
		}
		if itemTags != nil {
			split := strings.Split(*itemTags, " ")
			item.Tags = &split
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
		log.Printf("Error preparing query: %v", err)
	}
	defer stmt.Close()

	err = stmt.QueryRow(from).Scan(&delegate)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Error getting row from query: %v", err)
		return from
	}
	if delegate.Valid {
		return delegate.String
	} else {
		return from
	}
}
