package handler

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	containersTmpl *template.Template
	imagesTmpl     *template.Template
)

func initTemplates() error {
	baseHTML, err := templateFS.ReadFile("templates/base.html")
	if err != nil {
		return err
	}

	containersTmpl = template.Must(template.New("base").Parse(string(baseHTML)))
	imagesTmpl = template.Must(template.New("base").Parse(string(baseHTML)))

	return nil
}
