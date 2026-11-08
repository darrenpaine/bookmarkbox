package main

import (
	"net/http"

	"github.com/darrenpaine/bookmarkbox/ui"
	"github.com/go-chi/chi/v5"
)

// The routes() method returns a servemux containing the application routes.
func (app *application) routes() http.Handler {
	app.router.Use(secureHeaders)
	app.router.Use(app.logRequest)
	app.router.Use(app.recoverPanic)

	fs := http.FileServer(http.FS(ui.Files))
	app.router.Handle("/static/*", cacheStatic(fs))
	app.router.Get("/ping", app.ping) // no session/user

	// Unprotected routes - not requiring log in
	app.router.Group(func(group_router chi.Router) {
		group_router.Use(app.sessionManager.LoadAndSave)
		group_router.Use(app.authenticate)
		group_router.Get("/", app.home)
		group_router.Get("/about", app.about)
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
		group_router.Get("/bookmarks/new", app.bookmarkCreateShowForm)
		group_router.Post("/bookmarks/new", app.bookmarkCreatePost)
		group_router.Get("/bookmarks/{id}", app.bookmarkView)
		group_router.Get("/bookmarks/{id}/edit", app.bookmarkEditShowForm)
		group_router.Post("/bookmarks/{id}/edit", app.bookmarkEditPost)
		group_router.Post("/bookmarks/{id}/favourite", app.bookmarkFavouritePost)
		group_router.Post("/bookmarks/{id}/read-later", app.bookmarkReadLaterPost)
		group_router.Post("/bookmarks/{id}/tags", app.bookmarkTagAddPost)
		group_router.Post("/bookmarks/{id}/tags/remove", app.bookmarkTagRemovePost)
		group_router.Post("/bookmarks/{id}/delete", app.bookmarkDeletePost)
		group_router.Get("/tags", app.tagList)
		group_router.Post("/tags/{id}/rename", app.tagRenamePost)
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
