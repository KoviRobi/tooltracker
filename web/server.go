package web

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/KoviRobi/tooltracker/db"
)

//go:embed stylesheet.css
var stylesheet_css []byte

//go:embed tool.html
var tool_html string

//go:embed tracker.html
var tracker_html string

type server struct {
	db db.DB
}

const maxImageSize = 100 * 1024

func hideEmail(email string) string {
	split := strings.SplitN(email, "@", 2)
	if len(split) != 2 {
		// Malformed
		return email
	}
	user := split[0]
	domain := split[1]

	if len(user) < 6 {
		return user
	}

	return fmt.Sprintf("%.6s...@%s", user, domain)
}

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func serveStylesheet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Write(stylesheet_css)
}

func (server *server) serveTracker(w http.ResponseWriter, r *http.Request) {
	var writer bytes.Buffer
	items := server.db.GetItems()
	err := getTracker(&writer, items)
	if err != nil {
		serveError(w, err)
	} else {
		w.Write(writer.Bytes())
	}
}

func getTracker(w io.Writer, dbItems []db.Item) error {
	t, err := template.New("tracker").Parse(tracker_html)
	if err != nil {
		return err
	}

	type Item struct {
		Tool        string
		Description string
		LastSeenBy  string
		Comment     string
	}

	var items []Item
	for _, dbItem := range dbItems {
		item := Item{Tool: dbItem.Tool}

		if dbItem.Alias != nil {
			item.LastSeenBy = *dbItem.Alias
		} else {
			item.LastSeenBy = hideEmail(dbItem.LastSeenBy)
		}

		if dbItem.Description != nil {
			item.Description = *dbItem.Description
		}

		if dbItem.Comment != nil {
			item.Comment = *dbItem.Comment
		}

		items = append(items, item)
	}

	return t.Execute(w, items)
}

func (server *server) serveTool(w http.ResponseWriter, r *http.Request) {
	var writer bytes.Buffer

	name := r.URL.Query().Get("name")
	if name == "" {
		serveError(w, errors.New("Tool name missing"))
		return
	}

	tool := server.db.GetTool(name)
	if tool.Name == "" {
		tool.Name = name
	}
	if tool.Description == nil {
		empty := ""
		tool.Description = &empty
	}

	if r.Method == "POST" {
		// Limit size
		r.Body = http.MaxBytesReader(w, r.Body, (1+100)*1024)

		description := strings.TrimSpace(r.FormValue("description"))
		if description != "" {
			tool.Description = &description
		}

		file, hdr, err := r.FormFile("image")
		if err != nil && err != http.ErrMissingFile {
			log.Fatal(err)
		}

		if hdr != nil {
			defer file.Close()

			tool.Image = make([]byte, maxImageSize)
			n, err := file.Read(tool.Image)
			tool.Image = tool.Image[:n]
			if err != nil {
				log.Fatal(err)
			}
		}

		server.db.UpdateTool(tool)
	}

	err := getTool(&writer, tool)
	if err != nil {
		serveError(w, err)
	} else {
		w.Write(writer.Bytes())
	}
}

func getTool(w io.Writer, dbTool db.Tool) error {
	t, err := template.New("tool").Parse(tool_html)
	if err != nil {
		return err
	}

	type Tool struct {
		Name        string
		Description string
		Image       string
	}

	tool := Tool{Name: dbTool.Name}
	if dbTool.Description != nil {
		tool.Description = *dbTool.Description
	}

	if len(dbTool.Image) > 0 {
		tool.Image = base64.StdEncoding.EncodeToString(dbTool.Image)
	}

	return t.Execute(w, tool)
}

func redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/tracker", http.StatusTemporaryRedirect)
	return
}

func Serve(db db.DB) error {
	server := server{
		db: db,
	}

	http.HandleFunc("/stylesheet.css", serveStylesheet)
	http.HandleFunc("/tool", server.serveTool)
	http.HandleFunc("/tracker", server.serveTracker)
	http.HandleFunc("/", redirect)

	// TODO: Don't hardcode
	return http.ListenAndServe("localhost:8123", nil)
}
