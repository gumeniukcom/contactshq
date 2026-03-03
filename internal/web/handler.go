package web

import (
	"html/template"
	"io/fs"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

func RegisterRoutes(app *fiber.App) {
	// Landing page
	tmpl, err := template.ParseFS(TemplateFiles, "templates/landing.html")
	if err != nil {
		panic("failed to parse landing template: " + err.Error())
	}

	app.Get("/", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return tmpl.Execute(c.Response().BodyWriter(), nil)
	})

	// SPA — serve from embedded filesystem
	spaFS, err := fs.Sub(SPAFiles, "static/spa")
	if err != nil {
		panic("failed to create SPA sub-filesystem: " + err.Error())
	}

	app.Use("/app", filesystem.New(filesystem.Config{
		Root:         http.FS(spaFS),
		Browse:       false,
		NotFoundFile: "index.html",
	}))
}
