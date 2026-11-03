package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"snippetbox.darrenpaine.com/ui"
)

// The routes() method returns a servemux containing the application routes.
func (app *application) routes() http.Handler {
	app.router.Use(secureHeaders)
	app.router.Use(app.logRequest)
	app.router.Use(app.recoverPanic)

	fs := http.FileServer(http.FS(ui.Files))
	app.router.Handle("/static/*", fs)
	app.router.Get("/ping", app.ping) // no session/user

	// Unprotected routes - not requiring log in
	app.router.Group(func(group_router chi.Router) {
		group_router.Use(app.sessionManager.LoadAndSave)
		group_router.Use(app.authenticate)
		group_router.Get("/", app.home)
		group_router.Get("/about", app.about)
		group_router.Get("/snippet/view/{id}", app.snippetView)
		group_router.Get("/user/signup", app.userSignupShowForm)
		group_router.Post("/user/signup", app.userSignupPost)
		group_router.Get("/user/login", app.userLoginShowForm)
		group_router.Post("/user/login", app.userLoginPost)
	})

	// Protected routes - that need a user logged in
	app.router.Group(func(group_router chi.Router) {
		group_router.Use(app.sessionManager.LoadAndSave)
		group_router.Use(app.authenticate)
		group_router.Use(app.requireAuthentication)
		group_router.Get("/snippet/create", app.snippetCreateShowForm)
		group_router.Post("/snippet/create", app.snippetCreatePost)
		group_router.Post("/user/logout", app.userLogoutPost)
		group_router.Get("/account/view", app.userAccountView)
		group_router.Get("/account/password/update", app.userPasswordShowForm)
		group_router.Post("/account/password/update", app.userPasswordPost)
	})

	app.router.NotFound(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.notFound(w)
	}))
	return app.router
}
