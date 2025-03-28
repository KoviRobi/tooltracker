package web

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
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

const maxImageSize = 100 * 1024

// A simple regexp to match an URI
var uriRe = regexp.MustCompile(
	`([a-zA-Z][a-zA-Z0-9+.-]*):` + // Scheme
		`//([a-zA-Z][a-zA-Z0-9.+-]*(:[a-zA-Z][a-zA-Z0-9.+-]*)?@)?` + // Authority
		`[\[\]a-zA-Z0-9.:+_-]+` + // Host/IP4/IP6
		`(:[0-9]+)?` + // Port
		`(/[a-zA-Z0-9/.:+#%&?=_-]*)?`, // Path
)

func linkURI(maybeHTML any) template.HTML {
	ret := ""
	insecure := fmt.Sprint(maybeHTML)
	// Ok to match RE against input, it is just a state machine
	uris := uriRe.FindAllStringSubmatchIndex(insecure, -1)
	prev := 0
	for _, uri := range uris {
		start, end := uri[0], uri[1]
		ret += template.HTMLEscapeString(insecure[prev:start])
		ret += `<a href="` + template.HTMLEscapeString(insecure[start:end]) + `">`
		ret += template.HTMLEscapeString(insecure[start:end])
		ret += `</a>`
		prev = end
	}
	ret += template.HTMLEscapeString(insecure[prev:])
	return template.HTML(ret)
}

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

// Wrapper for things that can serve an error
type templateArgs struct {
	args    any
	server  *Server
	path    string
	content string
}
type serveFormatted func(http.ResponseWriter, *http.Request) (*templateArgs, error)

func (fn serveFormatted) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var t *template.Template
	tpl, err := fn(w, r)
	if err == nil {
		t, err = template.
			New(tpl.path).
			Funcs(template.FuncMap{
				"addtag":         tags.AddTag,
				"deltag":         tags.DelTag,
				"highlightLinks": linkURI,
			}).Parse(tpl.content)

		// Fetch errors
		select {
		case tpl.server.LastError = <-tpl.server.ErrorChan:
		default:
		}
	}
	// Passed to templates so untyped anyway, hence using `any`
	type serverTemplate struct {
		Value      any
		MailError  error
		HttpPrefix string
	}
	var writer bytes.Buffer
	if err == nil {
		// So that we don't partially write the template then encounter an error,
		// as HTTP writer isn't buffering
		err = t.Execute(&writer, serverTemplate{
			HttpPrefix: tpl.server.HttpPrefix,
			Value:      tpl.args,
			MailError:  tpl.server.LastError,
		})
	}
	if err == nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write(writer.Bytes())
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func serveStatic(contentType string, data []byte) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}
}

func (server *Server) getTracker(w http.ResponseWriter, r *http.Request) (*templateArgs, error) {
	// Process/normalize tags
	query := r.URL.Query()
	filter := tags.DefaultFilter
	if query.Has("tags") {
		filter = tags.NormalizeTags(query["tags"])
	}

	// Format page to buffer in case of error
	dbItems := server.Db.GetItems(filter)

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
		Filter: filter,
		Items:  items,
	}

	return &templateArgs{
		server:  server,
		path:    "tracker.html",
		content: tracker_html,
		args:    tracker,
	}, nil
}

func (server *Server) getTool(w http.ResponseWriter, r *http.Request) (*templateArgs, error) {
	name := r.URL.Query().Get("name")
	if name == "" {
		return nil, errors.New("Tool name missing")
	}

	dbTool := server.Db.GetTool(name)
	if dbTool.Name == "" {
		dbTool.Name = name
	}
	if dbTool.Description == nil {
		empty := ""
		dbTool.Description = &empty
	}

	if r.Method == "POST" {
		// Limit size
		var maxMemory int64 = 101 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

		r.ParseMultipartForm(maxMemory)
		hidden := r.FormValue(tags.Hidden) != ""
		dbTool.Tags = tags.NormalizeTags(r.Form["tags"])
		// Allow the user to hide by manually specifying hidden instead of the
		// checkbox
		if !hidden {
			_, hidden = dbTool.Tags[tags.Hidden]
		}
		if hidden {
			// User might have specified using checkbox, so store back
			dbTool.Tags[tags.Hidden] = tags.Any
		} else {
			delete(dbTool.Tags, tags.Hidden)
		}

		description := strings.TrimSpace(r.FormValue("description"))
		if description != "" {
			dbTool.Description = &description
		}

		file, hdr, err := r.FormFile("image")
		if err != nil && err != http.ErrMissingFile {
			return nil, fmt.Errorf("Error getting attached image: %v", err)
		}

		if hdr != nil {
			defer file.Close()

			imageBin := make([]byte, maxImageSize)
			n, err := file.Read(imageBin)
			imageBin = imageBin[:n]
			dbTool.Image = base64.StdEncoding.EncodeToString(imageBin)
			if err != nil {
				return nil, fmt.Errorf("Error base64 encoding image %v", err)
			}
		}

		server.Db.UpdateTool(dbTool)
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
		return nil, fmt.Errorf("Error making QR code %s: %v", link, err)
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

	return &templateArgs{
		server:  server,
		path:    "tool.html",
		content: tool_html,
		args:    tool,
	}, err
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
	http.HandleFunc(server.HttpPrefix+"/", server.redirect)

	http.Handle(server.HttpPrefix+"/tool", serveFormatted(server.getTool))
	http.Handle(server.HttpPrefix+"/tracker", serveFormatted(server.getTracker))

	go func() {
		<-server.ShutdownChan
		httpServer.Close()
	}()

	return httpServer.ListenAndServe()
}
