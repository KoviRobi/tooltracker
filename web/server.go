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
	"net/url"
	"regexp"
	"strings"

	"github.com/KoviRobi/tooltracker/artwork"
	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/limits"
	"github.com/skip2/go-qrcode"
)

//go:embed stylesheet.css
var stylesheet_css []byte

//go:embed tool.html
var tool_html string

//go:embed tracker.html
var tracker_html string

var tagsRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_]+`)

type Server struct {
	Db         db.DB
	FromRe     *regexp.Regexp
	To         string
	Domain     string
	HttpPrefix string
}

// Passed to templates so untyped anyway, hence using `any`
type serverTemplate struct {
	HttpPrefix string
	Value      any
}

func (server *Server) templateArg(arg any) serverTemplate {
	return serverTemplate{HttpPrefix: server.HttpPrefix, Value: arg}
}

const maxImageSize = 100 * 1024

func (server *Server) hideEmail(email string) string {
	split := strings.SplitN(email, "@", 2)
	if len(split) != 2 {
		// Malformed
		return email
	}
	user := split[0]
	domain := split[1]

	if server.FromRe.FindStringIndex(email) != nil {
		return user
	}

	if len(user) < 6 {
		return user
	}

	return fmt.Sprintf("%.6s...@%s", user, domain)
}

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func serveStatic(contentType string, data []byte) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}
}

func (server *Server) serveTracker(w http.ResponseWriter, r *http.Request) {
	var writer bytes.Buffer
	tagsStr := r.URL.Query().Get("tags")
	tags := tagsRe.FindAllString(tagsStr, -1)
	items := server.Db.GetItems(tags)
	err := server.getTracker(&writer, items, tags)
	if err != nil {
		serveError(w, err)
	} else {
		w.Write(writer.Bytes())
	}
}

func (server *Server) getTracker(w io.Writer, dbItems []db.Item, filter []string) error {
	t, err := template.New("tracker").Parse(tracker_html)
	if err != nil {
		return err
	}

	type Item struct {
		Tool        string
		Tags        []string
		Description string
		LastSeenBy  string
		Comment     string
	}

	var items []Item

	for _, dbItem := range dbItems {
		item := Item{Tool: dbItem.Tool}

		if dbItem.Tags != nil {
			item.Tags = *dbItem.Tags
		}

		if dbItem.Alias != nil {
			item.LastSeenBy = *dbItem.Alias
		} else {
			item.LastSeenBy = server.hideEmail(dbItem.LastSeenBy)
		}

		if dbItem.Description != nil {
			item.Description = *dbItem.Description
		}

		if dbItem.Comment != nil {
			item.Comment = *dbItem.Comment
		}

		items = append(items, item)
	}

	type Tracker struct {
		Items  []Item
		Filter string
	}
	tracker := Tracker{
		Items:  items,
		Filter: strings.Join(filter, " "),
	}

	return t.Execute(w, server.templateArg(tracker))
}

func (server *Server) serveTool(w http.ResponseWriter, r *http.Request) {
	var writer bytes.Buffer

	name := r.URL.Query().Get("name")
	if name == "" {
		serveError(w, errors.New("Tool name missing"))
		return
	}

	tool := server.Db.GetTool(name)
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

		tool.Tags = tagsRe.FindAllString(r.FormValue("tags"), -1)

		description := strings.TrimSpace(r.FormValue("description"))
		if description != "" {
			tool.Description = &description
		}

		file, hdr, err := r.FormFile("image")
		if err != nil && err != http.ErrMissingFile {
			log.Fatalf("Error getting attached image: %v", err)
		}

		if hdr != nil {
			defer file.Close()

			imageBin := make([]byte, maxImageSize)
			n, err := file.Read(imageBin)
			imageBin = imageBin[:n]
			tool.Image = base64.StdEncoding.EncodeToString(imageBin)
			if err != nil {
				log.Fatal("Error base64 encoding image #v", err)
			}
		}

		server.Db.UpdateTool(tool)
	}

	err := server.getTool(&writer, tool)
	if err != nil {
		serveError(w, err)
	} else {
		w.Write(writer.Bytes())
	}
}

func (server *Server) getTool(w io.Writer, dbTool db.Tool) error {
	t, err := template.New("tool").Parse(tool_html)
	if err != nil {
		return err
	}

	type Tool struct {
		Name        string
		Tags        string
		Description string
		Image       string
		QR          string
		Link        string
	}

	link := fmt.Sprintf("mailto:%s@%s?subject=%s",
		url.QueryEscape(server.To),
		url.QueryEscape(server.Domain),
		url.QueryEscape("Borrowed "+dbTool.Name),
	)
	qr, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		log.Fatalf("Error making QR code %s: %v", link, err)
	}
	tool := Tool{
		Name:  dbTool.Name,
		Tags:  strings.Join(dbTool.Tags, " "),
		QR:    base64.StdEncoding.EncodeToString(qr),
		Link:  link,
		Image: dbTool.Image,
	}
	if dbTool.Description != nil {
		tool.Description = *dbTool.Description
	}

	return t.Execute(w, server.templateArg(tool))
}

func (server *Server) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, server.HttpPrefix+"/tracker", http.StatusTemporaryRedirect)
	return
}

func (server *Server) Serve(listen string) error {
	httpServer := http.Server{
		Addr:         listen,
		ReadTimeout:  limits.ReadTimeout,
		WriteTimeout: limits.WriteTimeout,
	}
	http.HandleFunc(server.HttpPrefix+"/stylesheet.css", serveStatic("text/css; charset=utf-8", stylesheet_css))
	http.HandleFunc(server.HttpPrefix+"/favicon.ico", serveStatic("image/x-icon", artwork.Favicon_ico))
	http.HandleFunc(server.HttpPrefix+"/logo.svg", serveStatic("image/svg+xml", artwork.Favicon_ico))
	http.HandleFunc(server.HttpPrefix+"/tool", server.serveTool)
	http.HandleFunc(server.HttpPrefix+"/tracker", server.serveTracker)
	http.HandleFunc(server.HttpPrefix+"/", server.redirect)

	return httpServer.ListenAndServe()
}
