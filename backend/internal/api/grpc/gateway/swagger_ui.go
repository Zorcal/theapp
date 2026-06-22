package gateway

import (
	"html/template"
	"net/http"

	_ "embed"
)

//go:embed templates/swagger-ui.html
var swaggerUITemplateFile string

var swaggerUITmpl = template.Must(template.New("swagger-ui").Parse(swaggerUITemplateFile))

type swaggerUISpec struct {
	Name string
	URL  string
}

type swaggerUIData struct {
	Title       string
	Specs       []swaggerUISpec
	PrimaryName string
}

func swaggerUIHandler(title, primaryName string, specs []swaggerUISpec) http.HandlerFunc {
	data := swaggerUIData{
		Title:       title,
		Specs:       specs,
		PrimaryName: primaryName,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := swaggerUITmpl.Execute(w, data); err != nil {
			http.Error(w, "failed to render docs", http.StatusInternalServerError)
		}
	}
}
