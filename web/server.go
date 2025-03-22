package web

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/skip2/go-qrcode"

	"github.com/KoviRobi/tooltracker/artwork"
	"github.com/KoviRobi/tooltracker/db"
	"github.com/KoviRobi/tooltracker/limits"
	"github.com/KoviRobi/tooltracker/tags"
)

//go:embed stylesheet.css
var stylesheet_css []byte

//go:embed tool.html
var tool_html string

//go:embed tracker.html
var tracker_html string

type Server struct {
	LastError    error
	Db           db.DB
	FromRe       *regexp.Regexp
	ErrorChan    chan error
	ShutdownChan chan struct{}
	To           string
	Domain       string
	HttpPrefix   string
}

// Passed to templates so untyped anyway, hence using `any`
type serverTemplate struct {
	Value      any
	Error      error
	HttpPrefix string
}

func (server *Server) templateArg(arg any) serverTemplate {
	select {
	case server.LastError = <-server.ErrorChan:
	default:
	}
	return serverTemplate{
		HttpPrefix: server.HttpPrefix,
		Value:      arg,
		Error:      server.LastError,
	}
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
	// Process/normalize tags
	query := r.URL.Query()
	newTags := tags.DefaultFilter
	if query.Has("tags") {
		newTags = tags.NormalizeTags(query["tags"])
	}

	// Format page to buffer in case of error
	items := server.Db.GetItems(newTags)
	var writer bytes.Buffer
	err := server.getTracker(&writer, items, newTags)

	if err != nil {
		serveError(w, err)
	} else {
		w.Header().Set("Content-Type", "text/html")
		w.Write(writer.Bytes())
	}
}

func (server *Server) getTracker(w io.Writer, dbItems []db.Item, filter tags.Tags) error {
	t, err := template.
		New("tracker").
		Funcs(template.FuncMap{
			"addtag": tags.AddTag,
			"deltag": tags.DelTag,
		}).
		Parse(tracker_html)
	if err != nil {
		return err
	}

	type Item struct {
		Tags        tags.Tags
		Tool        string
		Description string
		LastSeenBy  string
		Comment     string
	}

	var items []Item

	for _, dbItem := range dbItems {
		item := Item{Tool: dbItem.Tool}

		if dbItem.Tags != nil {
			item.Tags = tags.NormalizeTags(*dbItem.Tags)
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
		Filter tags.Tags
		Items  []Item
	}
	tracker := Tracker{
		Items:  items,
		Filter: filter,
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
		var maxMemory int64 = 101 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

		r.ParseMultipartForm(maxMemory)
		hidden := r.FormValue(tags.Hidden) != ""
		tool.Tags = tags.NormalizeTags(r.Form["tags"])
		// Allow the user to hide by manually specifying hidden instead of the
		// checkbox
		if !hidden {
			_, hidden = tool.Tags[tags.Hidden]
		}
		if hidden {
			// User might have specified using checkbox, so store back
			tool.Tags[tags.Hidden] = tags.Any
		} else {
			delete(tool.Tags, tags.Hidden)
		}

		description := strings.TrimSpace(r.FormValue("description"))
		if description != "" {
			tool.Description = &description
		}

		file, hdr, err := r.FormFile("image")
		if err != nil && err != http.ErrMissingFile {
			serveError(w, fmt.Errorf("Error getting attached image: %v", err))
			return
		}

		if hdr != nil {
			defer file.Close()

			imageBin := make([]byte, maxImageSize)
			n, err := file.Read(imageBin)
			imageBin = imageBin[:n]
			tool.Image = base64.StdEncoding.EncodeToString(imageBin)
			if err != nil {
				serveError(w, fmt.Errorf("Error base64 encoding image %v", err))
				return
			}
		}

		server.Db.UpdateTool(tool)
	}

	err := server.getTool(&writer, tool)
	if err != nil {
		serveError(w, err)
	} else {
		w.Header().Set("Content-Type", "text/html")
		w.Write(writer.Bytes())
	}
}

func (server *Server) getTool(w io.Writer, dbTool db.Tool) error {
	t, err := template.
		New("tool").
		Funcs(template.FuncMap{
			"addtag": tags.AddTag,
			"deltag": tags.DelTag,
		}).
		Parse(tool_html)
	if err != nil {
		return err
	}

	type Tool struct {
		Tags        tags.Tags
		Name        string
		Description string
		Image       string
		QR          string
		Link        string
		Hidden      bool
	}

	link := fmt.Sprintf("mailto:%s@%s?subject=%s",
		url.QueryEscape(server.To),
		url.QueryEscape(server.Domain),
		url.QueryEscape("Borrowed "+dbTool.Name),
	)
	qr, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		return fmt.Errorf("Error making QR code %s: %v", link, err)
	}
	tool := Tool{
		Name:  dbTool.Name,
		QR:    base64.StdEncoding.EncodeToString(qr),
		Link:  link,
		Image: dbTool.Image,
		Tags:  dbTool.Tags,
	}
	_, tool.Hidden = dbTool.Tags[tags.Hidden]
	// Remove so that the checkbox is the canonical source
	delete(tool.Tags, tags.Hidden)
	if dbTool.Description != nil {
		tool.Description = *dbTool.Description
	}

	return t.Execute(w, server.templateArg(tool))
}

func (server *Server) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, server.HttpPrefix+"/tracker", http.StatusTemporaryRedirect)
}

func (server *Server) Serve(listen string) error {
	httpServer := http.Server{
		Addr:         listen,
		ReadTimeout:  limits.ReadTimeout,
		WriteTimeout: limits.WriteTimeout,
	}
	http.HandleFunc(server.HttpPrefix+"/stylesheet.css", serveStatic("text/css; charset=utf-8", stylesheet_css))
	http.HandleFunc(server.HttpPrefix+"/favicon.ico", serveStatic("image/x-icon", artwork.Favicon_ico))
	http.HandleFunc(server.HttpPrefix+"/logo.svg", serveStatic("image/svg+xml", artwork.Logo_svg))
	http.HandleFunc(server.HttpPrefix+"/tool", server.serveTool)
	http.HandleFunc(server.HttpPrefix+"/tracker", server.serveTracker)
	http.HandleFunc(server.HttpPrefix+"/", server.redirect)

	go func() {
		<-server.ShutdownChan
		httpServer.Close()
	}()

	return httpServer.ListenAndServe()
}
