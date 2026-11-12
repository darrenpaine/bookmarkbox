package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/darrenpaine/bookmarkbox/internal/models"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Everything, fonts and scripts included, is served by BookmarkBox.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")
		next.ServeHTTP(w, r)
	})
}

// hsts tells browsers to use only HTTPS for this site for a year, so even a
// first visit typed without https:// can't be intercepted. Only the demo
// sends it: a self-hoster on plain HTTP would be locked out, so they set it
// at their proxy. It leaves out includeSubDomains, which would cover every
// other site on the host's domain.
func hsts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health checks call /ping every few seconds, which would flood the log.
		if r.URL.Path != "/ping" {
			app.logger.Info("request", "ip", app.clientIP(r), "proto", r.Proto, "method", r.Method, "uri", r.URL.RequestURI())
		}
		next.ServeHTTP(w, r)
	})
}
func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a deferred function (which will always be run in the event
		// of a panic as Go unwinds the stack).
		defer func() {
			// Use the builtin recover function to check if there has been a
			// panic or not. If there has...
			if err := recover(); err != nil {
				// Set a "Connection: close" header on the response.
				w.Header().Set("Connection", "close")
				// Call the app.serverError helper method to return a 500
				// Internal Server response.
				app.serverError(w, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Redirect any unauthenticated page access to user login
func (app *application) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the user is not authenticated, redirect to login and stop processing
		if !app.isAuthenticated(r) {
			// Store our current target, so we can redirect after login
			app.sessionManager.Put(r.Context(), "target", r.URL.Path)
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
			return
		}
		// Otherwise set the "Cache-Control: no-store" header so that pages
		// require authentication are not stored in the users browser cache (or
		// other intermediary cache).
		w.Header().Add("Cache-Control", "no-store")
		// And call the next handler in the chain.
		next.ServeHTTP(w, r)
	})
}

// Add authentication status to the context
func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the authenticated user id, or 0 means no user authenticated
		id := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
		if id == 0 {
			next.ServeHTTP(w, r)
			return
		}
		// Is that user still on the system? The same lookup gives every
		// page their name and theme.
		viewer, err := app.users.Viewer(id)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, err)
			return
		}
		if viewer != nil {
			ctx := context.WithValue(r.Context(), isAuthenticatedContextKey, true)
			ctx = context.WithValue(ctx, viewerContextKey, viewer)
			r = r.WithContext(ctx)
		}
		// Call the next handler in the chain.
		next.ServeHTTP(w, r)
	})
}

// cacheStatic lets browsers keep static files. Embedded files have no
// modification time, so without this every page load fetched the CSS and
// fonts again, and text was redrawn when the fonts arrived. Links from pages
// carry ?v=staticVersion, and the CSS names fonts by file name alone, so a
// changed font must get a new file name.
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") == staticVersion || strings.HasPrefix(r.URL.Path, "/static/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}
