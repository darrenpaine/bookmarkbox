package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/bookmarkfile"
	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/internal/validator"
)

// importTimeout is how long an upload, an import or an export may take,
// in place of the server's usual few seconds: a large bookmark file takes a
// while to send over a slow connection, and a large import to save.
const importTimeout = 2 * time.Minute

// importForm is the upload form. Its only field is the file.
type importForm struct {
	validator.Validator
}

// importPreview is what the preview page shows about an uploaded file.
type importPreview struct {
	Filename string
	Skipped  int // links that aren't web addresses, or are too long
	Merged   int // links repeated in the file
	Summary  *models.ImportSummary
}

// allowMoreTime extends the server's time limits for this request.
// Failing to is harmless: the request then has the usual time.
func allowMoreTime(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(importTimeout)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
}

// readImportFile reads an uploaded bookmark file and makes it ready to
// import, or says what is wrong with it.
func readImportFile(data []byte) (*models.ImportFile, string) {
	links, err := bookmarkfile.Parse(bytes.NewReader(data), models.MaxImportLinks)
	if errors.Is(err, bookmarkfile.ErrTooManyLinks) {
		return nil, fmt.Sprintf("This file has more than %d links, the most one import can take", models.MaxImportLinks)
	}
	if err != nil {
		return nil, "This file couldn't be read as a bookmark file"
	}
	f := models.PrepareImport(links, time.Now())
	if len(f.Items) == 0 {
		if f.Skipped > 0 {
			return nil, "None of the links in this file are web addresses that can be imported"
		}
		return nil, "No bookmarks were found in this file. Choose the HTML file your browser exported."
	}
	return f, ""
}

// Show the upload form, or the preview of a file already uploaded.
func (app *application) importShow(w http.ResponseWriter, r *http.Request) {
	userID := app.authenticatedUserID(r)
	data := app.newTemplateData(r)
	data.Form = importForm{}

	pending, err := app.imports.Get(userID)
	if errors.Is(err, models.ErrNoRecord) {
		app.render(w, http.StatusOK, "import.tmpl", data)
		return
	}
	if err != nil {
		app.serverError(w, err)
		return
	}
	f, problem := readImportFile(pending.Data)
	if problem != "" {
		// Checked on upload, so only a change to the rules gets here
		if err = app.imports.Delete(userID); err != nil {
			app.serverError(w, err)
			return
		}
		data.Form = importForm{validator.Validator{NonFieldErrors: []string{problem}}}
		app.render(w, http.StatusOK, "import.tmpl", data)
		return
	}
	summary, err := app.bookmarks.PreviewImport(userID, f.Items)
	if err != nil {
		app.serverError(w, err)
		return
	}
	data.Import = &importPreview{Filename: pending.Filename, Skipped: f.Skipped, Merged: f.Merged, Summary: summary}
	app.render(w, http.StatusOK, "import.tmpl", data)
}

// Upload a bookmark file, keep it, and show its preview. Nothing is imported
// until the User confirms.
func (app *application) importUploadPost(w http.ResponseWriter, r *http.Request) {
	allowMoreTime(w)
	// Room for the form's own parts besides the file
	r.Body = http.MaxBytesReader(w, r.Body, models.MaxImportBytes+64<<10)

	form := importForm{}
	file, header, err := r.FormFile("file")
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		form.AddFieldError("file", fmt.Sprintf("This file is too large: the most one import can take is %d MB", models.MaxImportBytes>>20))
	case errors.Is(err, http.ErrMissingFile):
		form.AddFieldError("file", "Choose a bookmark file")
	case err != nil:
		app.clientError(w, http.StatusBadRequest)
		return
	}

	var data []byte
	if file != nil {
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, models.MaxImportBytes+1))
		switch {
		case err != nil:
			app.serverError(w, err)
			return
		case len(data) > models.MaxImportBytes:
			form.AddFieldError("file", fmt.Sprintf("This file is too large: the most one import can take is %d MB", models.MaxImportBytes>>20))
		default:
			if _, problem := readImportFile(data); problem != "" {
				form.AddFieldError("file", problem)
			}
		}
	}
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, http.StatusUnprocessableEntity, "import.tmpl", data)
		return
	}

	// Just the name, whichever separator the uploader's system uses
	filename := header.Filename
	filename = strings.TrimSpace(filename[strings.LastIndexAny(filename, `/\`)+1:])
	if filename == "" {
		filename = "bookmarks.html"
	}
	filename = truncate(filename, 200)
	if err = app.imports.Save(app.authenticatedUserID(r), filename, data); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/account/import", http.StatusSeeOther)
}

// Import the uploaded file, all or nothing.
func (app *application) importConfirmPost(w http.ResponseWriter, r *http.Request) {
	allowMoreTime(w)
	userID := app.authenticatedUserID(r)
	pending, err := app.imports.Get(userID)
	if errors.Is(err, models.ErrNoRecord) {
		app.flashError(r, "That upload has expired, or was already imported. Choose the file again to import it.")
		http.Redirect(w, r, "/account/import", http.StatusSeeOther)
		return
	}
	if err != nil {
		app.serverError(w, err)
		return
	}
	f, problem := readImportFile(pending.Data)
	if problem != "" {
		app.flashError(r, problem+".")
		http.Redirect(w, r, "/account/import", http.StatusSeeOther)
		return
	}

	summary, err := app.bookmarks.Import(userID, f.Items)
	if errors.Is(err, models.ErrBookmarkLimit) || errors.Is(err, models.ErrTagLimit) {
		// Back to the preview, which explains
		app.flashError(r, "Nothing was imported, because this import would pass one of your limits.")
		http.Redirect(w, r, "/account/import", http.StatusSeeOther)
		return
	}
	if err != nil {
		app.serverError(w, err)
		return
	}
	if err = app.imports.Delete(userID); err != nil {
		app.serverError(w, err)
		return
	}

	message := "Imported " + plural(summary.New, "new bookmark", "new bookmarks") + "."
	if summary.Updated > 0 {
		message += " " + plural(summary.Updated, "bookmark you already had", "bookmarks you already had") + " gained tags."
	}
	app.flash(r, message)
	http.Redirect(w, r, "/bookmarks", http.StatusSeeOther)
}

// Throw away the uploaded file, importing nothing.
func (app *application) importCancelPost(w http.ResponseWriter, r *http.Request) {
	if err := app.imports.Delete(app.authenticatedUserID(r)); err != nil {
		app.serverError(w, err)
		return
	}
	app.flash(r, "Import cancelled. Nothing was saved.")
	http.Redirect(w, r, "/account/view", http.StatusSeeOther)
}

// Download every Bookmark as a bookmark file, which browsers and BookmarkBox
// can import.
func (app *application) exportGet(w http.ResponseWriter, r *http.Request) {
	allowMoreTime(w)
	bookmarks, err := app.bookmarks.All(app.authenticatedUserID(r))
	if err != nil {
		app.serverError(w, err)
		return
	}
	var buf bytes.Buffer
	if err = bookmarkfile.Write(&buf, models.ExportLinks(bookmarks)); err != nil {
		app.serverError(w, err)
		return
	}
	filename := "bookmarkbox-" + time.Now().Format("2006-01-02") + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}

// truncate shortens s to at most n characters.
func truncate(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// plural gives a count with the right form of a noun: "1 bookmark", "2 bookmarks".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
