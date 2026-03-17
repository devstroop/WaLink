package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// pageLayout maps each page template to its layout.
var pageLayout = map[string]string{
	"dashboard":      "base",
	"accounts":       "base",
	"account-detail": "base",
	"users":          "base",
	"roles":          "base",
	"api-keys":       "base",
	"settings":       "base",
	"login":           "auth",
	"register":        "auth",
	"forgot-password": "auth",
	"reset-password":  "auth",
	"error":           "auth",
}

// Renderer compiles and executes HTML templates.
type Renderer struct {
	pages map[string]*template.Template
}

// NewRenderer pre-compiles all page template sets.
func NewRenderer() *Renderer {
	r := &Renderer{pages: make(map[string]*template.Template)}
	r.buildAll()
	return r
}

func (r *Renderer) buildAll() {
	for name, layout := range pageLayout {
		r.pages[name] = r.buildPage(name, layout)
	}
	log.Debug().Int("pages", len(r.pages)).Msg("web templates compiled")
}

func (r *Renderer) buildPage(name, layout string) *template.Template {
	t := template.New("").Funcs(funcMap())

	// 1. Layout
	t = template.Must(t.ParseFS(templateFS,
		"templates/layouts/"+layout+".html"))

	// 2. Partials
	if m, _ := fs.Glob(templateFS, "templates/partials/*.html"); len(m) > 0 {
		t = template.Must(t.ParseFS(templateFS, "templates/partials/*.html"))
	}

	// 3. Components
	if m, _ := fs.Glob(templateFS, "templates/components/*.html"); len(m) > 0 {
		t = template.Must(t.ParseFS(templateFS, "templates/components/*.html"))
	}

	// 4. Page
	t = template.Must(t.ParseFS(templateFS,
		"templates/pages/"+name+".html"))

	return t
}

// Page renders a full page.
func (r *Renderer) Page(w http.ResponseWriter, status int, page string, data PageData) {
	t, ok := r.pages[page]
	if !ok {
		log.Error().Str("page", page).Msg("template not found")
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Str("page", page).Msg("template render failed")
	}
}

// Partial renders a named partial/component fragment (for HTMX responses).
func (r *Renderer) Partial(w http.ResponseWriter, status int, page string, block string, data any) {
	t, ok := r.pages[page]
	if !ok {
		log.Error().Str("page", page).Msg("template not found for partial")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, block, data); err != nil {
		log.Error().Err(err).Str("block", block).Msg("partial render failed")
	}
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				k, _ := pairs[i].(string)
				m[k] = pairs[i+1]
			}
			return m
		},
		"list": func(items ...any) []any {
			return items
		},
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"timeAgo":   timeAgo,
		"initial": func(s string) string {
			for _, r := range s {
				return string(r)
			}
			return ""
		},
		"truncate": func(v any, n int) string {
			var s string
			switch val := v.(type) {
			case string:
				s = val
			case *string:
				if val == nil {
					return ""
				}
				s = *val
			default:
				s = fmt.Sprint(v)
			}
			if len(s) <= n {
				return s
			}
			return s[:n] + "…"
		},
		"add": func(a, b int) int { return a + b },
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
	}
}

func timeAgo(v any) string {
	var s string
	switch val := v.(type) {
	case string:
		s = val
	case *string:
		if val == nil {
			return ""
		}
		s = *val
	default:
		return fmt.Sprint(v)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
