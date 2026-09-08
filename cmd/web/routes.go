package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// The routes() method returns a servemux containing our application routes.
func (app *application) routes() http.Handler {
	app.router.Use(secureHeaders)
	app.router.Use(app.logRequest)
	app.router.Use(app.recoverPanic)

	fs := http.FileServer(http.Dir("./ui/static"))
	app.router.Handle("/static/*", http.StripPrefix("/static/", fs))

	app.router.Group(func(group_router chi.Router) {
		group_router.Use(app.sessionManager.LoadAndSave)
		group_router.Get("/", app.home)
		group_router.Get("/snippet/view/{id}", app.snippetView)
		group_router.Get("/snippet/create", app.snippetCreateShowForm)
		group_router.Post("/snippet/create", app.snippetCreatePost)
	})

	app.router.NotFound(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.notFound(w)
	}))
	return app.router
}
