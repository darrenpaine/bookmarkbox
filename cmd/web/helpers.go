package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/go-playground/form/v4"
)

// Largest form post accepted.
const maxFormBytes = 1 << 20 // 1 MB

// The serverError helper logs the error with a stack trace, and
// then sends a generic 500 response (or the trace itself in debug mode).
func (app *application) serverError(w http.ResponseWriter, err error) {
	stack := debug.Stack()
	app.logger.Error(err.Error(), "stack", string(stack))
	if app.debug {
		http.Error(w, fmt.Sprintf("%s\n%s", err, stack), http.StatusInternalServerError)
		return
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// Helper to send back error responses.
func (app *application) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// Convenience 404 wrapper
func (app *application) notFound(w http.ResponseWriter) {
	app.clientError(w, http.StatusNotFound)
}

// Helper to render a given page template.
func (app *application) render(w http.ResponseWriter, status int, page string, data *templateData) {
	// Retrieve the appropriate template set from the cache.
	ts, ok := app.templateCache[page]
	if !ok {
		err := fmt.Errorf("the template %s does not exist", page)
		app.serverError(w, err)
		return
	}
	// Initialize a new buffer.
	buf := new(bytes.Buffer)
	// Write the template to the buffer to check for errors.
	err := ts.ExecuteTemplate(buf, "base", data)
	if err != nil {
		app.serverError(w, err)
		return
	}
	// Write out the provided HTTP status code.
	w.WriteHeader(status)
	// Write the output
	_, err = buf.WriteTo(w)
	if err != nil {
		app.serverError(w, err)
		return
	}
}

// renderFragment renders one named template on its own, for htmx to swap
// into the page. Every page's template set has the partials, so any one will do.
func (app *application) renderFragment(w http.ResponseWriter, status int, name string, data any) {
	buf := new(bytes.Buffer)
	if err := app.templateCache["home.tmpl"].ExecuteTemplate(buf, name, data); err != nil {
		app.serverError(w, err)
		return
	}
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// isHTMX reports whether htmx made the request, so it wants a fragment back.
// When htmx restores a page for Back, it wants the whole page.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"
}

// flash shows a success message on the next page.
func (app *application) flash(r *http.Request, message string) {
	app.sessionManager.Put(r.Context(), "flash", message)
}

// flashError shows an error message on the next page.
func (app *application) flashError(r *http.Request, message string) {
	app.sessionManager.Put(r.Context(), "flashError", message)
}

// safeNext returns next if it is a path on this site, otherwise fallback, so a
// form can't redirect somewhere else.
func safeNext(next, fallback string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return fallback
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return fallback
	}
	return next
}

// Helper to initialise template data.
func (app *application) newTemplateData(r *http.Request) *templateData {
	data := &templateData{
		CurrentYear:     time.Now().Year(),
		CurrentPath:     r.URL.Path,
		Flash:           app.sessionManager.PopString(r.Context(), "flash"),
		IsAuthenticated: app.isAuthenticated(r),
		Viewer:          viewer(r),
	}
	if message := app.sessionManager.PopString(r.Context(), "flashError"); message != "" {
		data.Flash = message
		data.FlashIsError = true
	}
	// Only logged-out pages show the Signup link. A failed check just hides it.
	if !data.IsAuthenticated {
		open, err := app.users.SignupOpen()
		if err != nil {
			app.logger.Error(err.Error())
		}
		data.SignupOpen = open
	}
	return data
}

// Helper to decode form data into the dst.
func (app *application) decodePostForm(w http.ResponseWriter, r *http.Request, dst any) error {
	// No form needs more than this, and without a limit a client could make
	// the server read up to Go's 10 MB form limit on every request.
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	err := r.ParseForm()
	if err != nil {
		return err
	}

	err = app.formDecoder.Decode(dst, r.PostForm)
	if err != nil {
		var invalidDecoderError *form.InvalidDecoderError
		if errors.As(err, &invalidDecoderError) {
			panic(err)
		}
		// For all other errors, we return them as normal.
		return err
	}
	return nil
}

// Return true if the request is from an authenticated user, else false
// viewer returns the logged-in User's name and theme, or nil when logged out.
func viewer(r *http.Request) *models.Viewer {
	v, _ := r.Context().Value(viewerContextKey).(*models.Viewer)
	return v
}

func (app *application) isAuthenticated(r *http.Request) bool {
	isAuthenticated, ok := r.Context().Value(isAuthenticatedContextKey).(bool)
	if !ok {
		return false
	}
	return isAuthenticated
}

// The ID of the logged-in user, or 0 if nobody is logged in.
func (app *application) authenticatedUserID(r *http.Request) int {
	return app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
}
