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
	"slices"
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
	Db           db.DB
	FromRe       *regexp.Regexp
	To           string
	Domain       string
	HttpPrefix   string
	ErrorChan    chan error
	LastError    error
	ShutdownChan chan struct{}
}

// Passed to templates so untyped anyway, hence using `any`
type serverTemplate struct {
	HttpPrefix string
	Value      any
	Error      error
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
	var writer bytes.Buffer
	tagsStr := tags.DefaultFilter
	if r.URL.Query().Has("tags") {
		tagsStr = r.URL.Query().Get("tags")
	}
	if r.URL.Query().Has("addtags") {
		tagsStr += " " + r.URL.Query().Get("addtags")
	}
	tags := tags.Re.FindAllString(tagsStr, -1)
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

		tool.Tags = tags.Re.FindAllString(r.FormValue("tags"), -1)
		hidden := r.FormValue("hidden") == ""
		if !hidden {
			tool.Tags = append(tool.Tags, tags.Hidden)
		}
		slices.Sort(tool.Tags)
		tool.Tags = slices.Compact(tool.Tags)
		if hidden {
			tool.Tags = slices.DeleteFunc(tool.Tags, func(tag string) bool { return tag == tags.Hidden })
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
		return fmt.Errorf("Error making QR code %s: %v", link, err)
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
