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

	"github.com/KoviRobi/tooltracker/db"
	"github.com/skip2/go-qrcode"
)

// generated with
//
//	$ magick \
//	  -density 300 \
//	  -define icon:auto-resize=64,48,32,16 \
//	  -background none \
//	  artwork/logo.svg \
//	  artwork/favicon.ico
//
//go:embed stylesheet.css
var stylesheet_css []byte

//go:embed artwork/favicon.ico
var favicon_ico []byte

//go:embed tool.html
var tool_html string

//go:embed tracker.html
var tracker_html string

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
	items := server.Db.GetItems()
	err := server.getTracker(&writer, items)
	if err != nil {
		serveError(w, err)
	} else {
		w.Write(writer.Bytes())
	}
}

func (server *Server) getTracker(w io.Writer, dbItems []db.Item) error {
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

	return t.Execute(w, server.templateArg(items))
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
		Description string
		Image       string
		QR          string
	}

	link := fmt.Sprintf("mailto:%s@%s?subject=%s",
		server.To,
		server.Domain,
		url.QueryEscape("Borrowed "+dbTool.Name),
	)
	qr, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		log.Fatal(err)
	}
	tool := Tool{
		Name: dbTool.Name,
		QR:   base64.StdEncoding.EncodeToString(qr),
	}
	if dbTool.Description != nil {
		tool.Description = *dbTool.Description
	}

	if len(dbTool.Image) > 0 {
		tool.Image = base64.StdEncoding.EncodeToString(dbTool.Image)
	}

	return t.Execute(w, server.templateArg(tool))
}

func (server *Server) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, server.HttpPrefix+"/tracker", http.StatusTemporaryRedirect)
	return
}

func (server *Server) Serve(listen string) error {
	http.HandleFunc(server.HttpPrefix+"/stylesheet.css", serveStatic("text/css; charset=utf-8", stylesheet_css))
	http.HandleFunc(server.HttpPrefix+"/favicon.ico", serveStatic("text/svg", favicon_ico))
	http.HandleFunc(server.HttpPrefix+"/tool", server.serveTool)
	http.HandleFunc(server.HttpPrefix+"/tracker", server.serveTracker)
	http.HandleFunc(server.HttpPrefix+"/", server.redirect)

	return http.ListenAndServe(listen, nil)
}
