package gateway

import (
	"html/template"
	"net/http"

	_ "embed"
)

//go:embed templates/swagger-ui.html
var swaggerUITemplateFile string

var swaggerUITmpl = template.Must(template.New("swagger-ui").Parse(swaggerUITemplateFile))

type swaggerUIData struct {
	Title   string
	SpecURL string
}

func swaggerUIHandler(title, specURL string) http.HandlerFunc {
	data := swaggerUIData{
		Title:   title,
		SpecURL: specURL,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := swaggerUITmpl.Execute(w, data); err != nil {
			http.Error(w, "failed to render docs", http.StatusInternalServerError)
		}
	}
}
