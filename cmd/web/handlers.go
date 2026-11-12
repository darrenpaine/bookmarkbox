package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/internal/validator"
	"github.com/go-chi/chi/v5"
)

// Bookmark add and edit form - with field validation
type bookmarkForm struct {
	validator.Validator `form:"-"`
	Action              string `form:"-"` // where the form posts to
	URL                 string `form:"url"`
	Title               string `form:"title"`
	Description         string `form:"description"`
	Notes               string `form:"notes"`
	Tags                string `form:"tags"` // comma-separated
	Favourite           bool   `form:"favourite"`
	ReadLater           bool   `form:"read_later"`
}

// Favourite, Read later and delete forms. Next is where to go back to when
// JavaScript is off.
type actionForm struct {
	Next string `form:"next"`
}

// Form to add or remove one tag on a bookmark's page.
type bookmarkTagForm struct {
	Name string `form:"name"`
}

// Tag rename form. Confirm is set by the merge confirmation page.
type tagRenameForm struct {
	Name    string `form:"name"`
	Confirm bool   `form:"confirm"`
}

// User Signup form - with field validation
type userSignupForm struct {
	validator.Validator `form:"-"`
	Name                string `form:"name"`
	Email               string `form:"email"`
	Password            string `form:"password"`
}

// User Login form - with validation
type userLoginForm struct {
	validator.Validator `form:"-"`
	Email               string `form:"email"`
	Password            string `form:"password"`
}

// User Change Password form - with validation
type userChangePasswordForm struct {
	validator.Validator `form:"-"`
	OldPassword         string `form:"old_password"`
	NewPassword         string `form:"new_password"`
	ConfirmPassword     string `form:"confirm_password"`
}

// The home page: a welcome when logged out, otherwise the list.
func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if app.isAuthenticated(r) {
		// The list, in the List view and sort remembered for the session.
		http.Redirect(w, r, "/bookmarks", http.StatusSeeOther)
		return
	}
	app.render(w, http.StatusOK, "home.tmpl", app.newTemplateData(r))
}

// Render the About page
func (app *application) about(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	app.render(w, http.StatusOK, "about.tmpl", data)
}

// tooLong is the validation message for a field over its length limit.
func tooLong(limit int) string {
	return fmt.Sprintf("This field cannot be more than %d characters long", limit)
}

// urlID reads the {id} URL parameter; ok is false if it isn't a positive integer.
func urlID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	return id, err == nil && id > 0
}

// Render one of the user's bookmarks. Other users' bookmarks are 404s.
func (app *application) bookmarkView(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	bookmark, err := app.bookmarks.Get(app.authenticatedUserID(r), id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}
	data := app.newTemplateData(r)
	data.Bookmark = bookmark
	data.BackURL = app.backURL(r)
	if !app.addTagSuggestions(w, r, data) {
		return
	}
	app.render(w, http.StatusOK, "view.tmpl", data)
}

// validate checks the form's fields, after trimming the URL and title.
func (form *bookmarkForm) validate() {
	form.URL = strings.TrimSpace(form.URL)
	form.Title = strings.TrimSpace(form.Title)

	form.CheckField(validator.NotBlank(form.URL), "url", "This field cannot be blank")
	form.CheckField(validator.WebURL(form.URL), "url", "This must be a full http:// or https:// address")
	form.CheckField(validator.MaxChars(form.URL, models.MaxURLChars), "url", tooLong(models.MaxURLChars))
	form.CheckField(validator.NotBlank(form.Title), "title", "This field cannot be blank")
	form.CheckField(validator.MaxChars(form.Title, models.MaxTitleChars), "title", tooLong(models.MaxTitleChars))
	form.CheckField(validator.MaxChars(form.Description, models.MaxDescriptionChars), "description", tooLong(models.MaxDescriptionChars))
	form.CheckField(validator.MaxChars(form.Notes, models.MaxNotesChars), "notes", tooLong(models.MaxNotesChars))
	for _, tag := range models.NormaliseTags(strings.Split(form.Tags, ",")) {
		form.CheckField(validator.MaxChars(tag, models.MaxTagNameChars), "tags", fmt.Sprintf("Each tag must be %d characters or fewer", models.MaxTagNameChars))
	}
}

// addSaveError shows a failed save on the form, and reports whether it could.
func (form *bookmarkForm) addSaveError(err error) bool {
	switch {
	case errors.Is(err, models.ErrDuplicateURL):
		form.AddFieldError("url", "You have already saved this address")
	case errors.Is(err, models.ErrTagLimit):
		form.AddFieldError("tags", "You have reached your limit of tags, so this can't add a new one")
	case errors.Is(err, models.ErrBookmarkLimit):
		form.AddNonFieldError("You have reached your limit of bookmarks")
	default:
		return false
	}
	return true
}

// bookmark builds the model value from the form's fields.
func (form *bookmarkForm) bookmark() *models.Bookmark {
	return &models.Bookmark{
		URL:         form.URL,
		Title:       form.Title,
		Description: form.Description,
		Notes:       form.Notes,
		Favourite:   form.Favourite,
		ReadLater:   form.ReadLater,
		Tags:        strings.Split(form.Tags, ","),
	}
}

// renderBookmarkForm shows the add or edit form, with Tag suggestions.
func (app *application) renderBookmarkForm(w http.ResponseWriter, r *http.Request, status int, page string, form bookmarkForm) {
	data := app.newTemplateData(r)
	data.Form = form
	if !app.addTagSuggestions(w, r, data) {
		return
	}
	app.render(w, status, page, data)
}

// renderBookmarkFormError re-displays a form with a validation error status.
func (app *application) renderBookmarkFormError(w http.ResponseWriter, r *http.Request, page string, form bookmarkForm) {
	app.renderBookmarkForm(w, r, http.StatusUnprocessableEntity, page, form)
}

// addTagSuggestions gives a page the User's Tags for autocomplete, most used
// first, then A-Z. It reports false after sending an error.
func (app *application) addTagSuggestions(w http.ResponseWriter, r *http.Request, data *templateData) bool {
	tags, err := app.tags.List(app.authenticatedUserID(r))
	if err != nil {
		app.serverError(w, err)
		return false
	}
	tags = slices.Clone(tags) // List gives them A-Z
	slices.SortStableFunc(tags, func(a, b *models.Tag) int { return b.BookmarkCount - a.BookmarkCount })
	data.TagSuggestions = tags
	return true
}

// Render the bookmark Create form.
func (app *application) bookmarkCreateShowForm(w http.ResponseWriter, r *http.Request) {
	app.renderBookmarkForm(w, r, http.StatusOK, "create.tmpl", bookmarkForm{Action: "/bookmarks/new"})
}

// Validate and save a new bookmark, then show it.
func (app *application) bookmarkCreatePost(w http.ResponseWriter, r *http.Request) {
	form := bookmarkForm{}
	err := app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.Action = "/bookmarks/new"

	form.validate()
	if !form.Valid() {
		app.renderBookmarkFormError(w, r, "create.tmpl", form)
		return
	}

	id, err := app.bookmarks.Insert(app.authenticatedUserID(r), form.bookmark())
	if err != nil {
		if form.addSaveError(err) {
			app.renderBookmarkFormError(w, r, "create.tmpl", form)
		} else {
			app.serverError(w, err)
		}
		return
	}

	app.flash(r, "Bookmark saved!")
	http.Redirect(w, r, fmt.Sprintf("/bookmarks/%d", id), http.StatusSeeOther)
}

// Render the edit form, filled in from one of the user's bookmarks.
func (app *application) bookmarkEditShowForm(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	bookmark, err := app.bookmarks.Get(app.authenticatedUserID(r), id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}
	app.renderBookmarkForm(w, r, http.StatusOK, "edit.tmpl", bookmarkForm{
		Action:      fmt.Sprintf("/bookmarks/%d/edit", id),
		URL:         bookmark.URL,
		Title:       bookmark.Title,
		Description: bookmark.Description,
		Notes:       bookmark.Notes,
		Tags:        strings.Join(bookmark.Tags, ", "),
		Favourite:   bookmark.Favourite,
		ReadLater:   bookmark.ReadLater,
	})
}

// Validate and save changes to a bookmark, then show it.
func (app *application) bookmarkEditPost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	form := bookmarkForm{}
	err := app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.Action = fmt.Sprintf("/bookmarks/%d/edit", id)

	form.validate()
	if !form.Valid() {
		app.renderBookmarkFormError(w, r, "edit.tmpl", form)
		return
	}

	bookmark := form.bookmark()
	bookmark.ID = id
	err = app.bookmarks.Update(app.authenticatedUserID(r), bookmark)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrNoRecord):
			app.notFound(w)
		case form.addSaveError(err):
			app.renderBookmarkFormError(w, r, "edit.tmpl", form)
		default:
			app.serverError(w, err)
		}
		return
	}

	app.flash(r, "Bookmark updated!")
	http.Redirect(w, r, fmt.Sprintf("/bookmarks/%d", id), http.StatusSeeOther)
}

// Toggle the favourite star. htmx gets the new star back; otherwise go back
// to the page the form was on.
func (app *application) bookmarkFavouritePost(w http.ResponseWriter, r *http.Request) {
	app.toggle(w, r, app.bookmarks.ToggleFavourite, "star")
}

// Toggle Read later, in the same way as the favourite star.
func (app *application) bookmarkReadLaterPost(w http.ResponseWriter, r *http.Request) {
	app.toggle(w, r, app.bookmarks.ToggleReadLater, "readlater")
}

// toggle runs one of the bookmark toggles, then returns the named fragment to
// htmx, or redirects.
func (app *application) toggle(w http.ResponseWriter, r *http.Request, change func(userID, id int) error, fragment string) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	var form actionForm
	if err := app.decodePostForm(w, r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	next := safeNext(form.Next, fmt.Sprintf("/bookmarks/%d", id))
	userID := app.authenticatedUserID(r)
	if err := change(userID, id); err != nil {
		app.modelError(w, err)
		return
	}
	if !isHTMX(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	bookmark, err := app.bookmarks.Get(userID, id)
	if err != nil {
		app.modelError(w, err)
		return
	}
	app.renderFragment(w, http.StatusOK, fragment, bookmarkItem{bookmark, next, nil})
}

// modelError sends a 404 for ErrNoRecord, and a 500 for anything else.
func (app *application) modelError(w http.ResponseWriter, err error) {
	if errors.Is(err, models.ErrNoRecord) {
		app.notFound(w)
	} else {
		app.serverError(w, err)
	}
}

// Delete a bookmark. When htmx deletes a row of a list, the empty response
// removes the row. Otherwise go to the home page with a message.
func (app *application) bookmarkDeletePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	var form actionForm
	if err := app.decodePostForm(w, r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if err := app.bookmarks.Delete(app.authenticatedUserID(r), id); err != nil {
		app.modelError(w, err)
		return
	}
	if isHTMX(r) && r.Header.Get("HX-Target") == fmt.Sprintf("bookmark-%d", id) {
		// The row goes, and on a list the count in its heading drops.
		if err := app.listTitle(w, r); err != nil {
			app.serverError(w, err)
		}
		return
	}
	app.flash(r, "Bookmark deleted.")
	next := safeNext(form.Next, "/")
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", next)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// tagName normalises a tag name typed by the user, or explains what's wrong.
// Commas split tags on the bookmark form, so they can't be in a name.
func tagName(raw string) (name, problem string) {
	names := models.NormaliseTags([]string{raw})
	switch {
	case len(names) == 0:
		return "", "A tag name cannot be blank"
	case strings.Contains(raw, ","):
		return "", "A tag name cannot contain a comma"
	case !validator.MaxChars(names[0], models.MaxTagNameChars):
		return "", fmt.Sprintf("A tag name must be %d characters or fewer", models.MaxTagNameChars)
	}
	return names[0], ""
}

// Add a tag to a bookmark from its page.
func (app *application) bookmarkTagAddPost(w http.ResponseWriter, r *http.Request) {
	app.changeTag(w, r, true)
}

// Remove a tag from a bookmark from its page.
func (app *application) bookmarkTagRemovePost(w http.ResponseWriter, r *http.Request) {
	app.changeTag(w, r, false)
}

// changeTag adds or removes one tag. htmx gets the bookmark's tag editor back,
// with any problem shown in it; otherwise the bookmark's page shows it.
func (app *application) changeTag(w http.ResponseWriter, r *http.Request, add bool) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	var form bookmarkTagForm
	if err := app.decodePostForm(w, r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	userID := app.authenticatedUserID(r)
	name, problem := tagName(form.Name)
	if problem == "" {
		var err error
		if add {
			err = app.bookmarks.AddTag(userID, id, name)
		} else {
			err = app.bookmarks.RemoveTag(userID, id, name)
		}
		switch {
		case errors.Is(err, models.ErrTagLimit):
			problem = "You have reached your limit of tags, so this can't add a new one"
		case err != nil:
			app.modelError(w, err)
			return
		}
	}

	if !isHTMX(r) {
		if problem != "" {
			app.flashError(r, problem)
		}
		http.Redirect(w, r, fmt.Sprintf("/bookmarks/%d", id), http.StatusSeeOther)
		return
	}
	bookmark, err := app.bookmarks.Get(userID, id)
	if err != nil {
		app.modelError(w, err)
		return
	}
	// htmx only swaps successful responses, so a problem is still a 200.
	app.renderFragment(w, http.StatusOK, "tageditor", &templateData{
		Bookmark:      bookmark,
		TagError:      problem,
		FocusTagInput: add,
	})
}

// Render the user's tags, with a rename box for each.
func (app *application) tagList(w http.ResponseWriter, r *http.Request) {
	app.renderTagList(w, r, http.StatusOK, app.newTemplateData(r))
}

// renderTagList loads the tags and renders the list with the given status.
func (app *application) renderTagList(w http.ResponseWriter, r *http.Request, status int, data *templateData) {
	tags, err := app.tags.List(app.authenticatedUserID(r))
	if err != nil {
		app.serverError(w, err)
		return
	}
	data.Tags = tags
	app.render(w, status, "tags.tmpl", data)
}

// Rename a tag. Renaming onto an existing tag shows a confirmation page, which
// posts back here with Confirm set to merge the two.
func (app *application) tagRenamePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	var form tagRenameForm
	err := app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	userID := app.authenticatedUserID(r)
	tag, err := app.tags.Get(userID, id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	name, problem := tagName(form.Name)
	if problem != "" {
		data := app.newTemplateData(r)
		data.Flash = problem
		data.FlashIsError = true
		app.renderTagList(w, r, http.StatusUnprocessableEntity, data)
		return
	}
	if name == tag.Name {
		http.Redirect(w, r, "/tags", http.StatusSeeOther)
		return
	}

	if form.Confirm {
		err = app.tags.Merge(userID, id, name)
	} else {
		err = app.tags.Rename(userID, id, name)
	}
	if err != nil {
		switch {
		case errors.Is(err, models.ErrDuplicateTag):
			data := app.newTemplateData(r)
			data.Tag = tag
			data.MergeInto = name
			app.render(w, http.StatusOK, "tagmerge.tmpl", data)
		case errors.Is(err, models.ErrNoRecord):
			app.notFound(w)
		default:
			app.serverError(w, err)
		}
		return
	}

	if form.Confirm {
		app.flash(r, "Tags merged.")
	} else {
		app.flash(r, "Tag renamed.")
	}
	http.Redirect(w, r, "/tags", http.StatusSeeOther)
}

// passwordTooLong explains the bcrypt limit. It counts bytes, so letters
// outside A to Z use up more than one.
const passwordTooLong = "The password must be 72 characters or fewer (fewer with accented letters or symbols)"

// Show a form to create a user
func (app *application) userSignupShowForm(w http.ResponseWriter, r *http.Request) {
	open, err := app.users.SignupOpen()
	if err != nil {
		app.serverError(w, err)
		return
	}
	if !open {
		app.render(w, http.StatusOK, "signupclosed.tmpl", app.newTemplateData(r))
		return
	}
	data := app.newTemplateData(r)
	data.Form = userSignupForm{}
	app.render(w, http.StatusOK, "signup.tmpl", data)
}

// Post method to create a user
func (app *application) userSignupPost(w http.ResponseWriter, r *http.Request) {
	open, err := app.users.SignupOpen()
	if err != nil {
		app.serverError(w, err)
		return
	}
	if !open {
		app.render(w, http.StatusForbidden, "signupclosed.tmpl", app.newTemplateData(r))
		return
	}
	var form userSignupForm
	err = app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if !app.allowAttempt(r) {
		form.AddNonFieldError(tooManyAttemptsMessage)
		app.tooManyAttempts(w, r, "signup.tmpl", form)
		return
	}
	// VALIDATION:
	// Note email field is case insensitive in the DB (utf8mb4_unicode_ci)
	form.CheckField(validator.NotBlank(form.Name), "name", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "Invalid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.Password, 8), "password", "The password must contain at least 8 characters")
	form.CheckField(validator.MaxBytes(form.Password, models.MaxPasswordBytes), "password", passwordTooLong)
	// form.CheckField(validator.UniqueEmail(form.Email), "email", "Email address already in use")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, http.StatusUnprocessableEntity, "signup.tmpl", data)
		return
	}
	// Try to create a new user record in the database. If the email already
	// exists then add an error message to the form and re-display it.
	err = app.users.Insert(form.Name, form.Email, form.Password)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrDuplicateEmail):
			form.AddFieldError("email", "Email address is already in use")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "signup.tmpl", data)
		case errors.Is(err, models.ErrUserLimit):
			// Someone else took the last place since the check above.
			app.render(w, http.StatusForbidden, "signupclosed.tmpl", app.newTemplateData(r))
		default:
			app.serverError(w, err)
		}
		return
	}
	// Otherwise add a confirmation flash message confirming that signup worked.
	app.flash(r, "Your signup was successful. Please log in.")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

// User login form
func (app *application) userLoginShowForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = userLoginForm{}
	app.render(w, http.StatusOK, "login.tmpl", data)
}

// Authenticate and log the user in
func (app *application) userLoginPost(w http.ResponseWriter, r *http.Request) {
	var form userLoginForm
	err := app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if !app.allowAttempt(r) {
		form.AddNonFieldError(tooManyAttemptsMessage)
		app.tooManyAttempts(w, r, "login.tmpl", form)
		return
	}
	// Do some validation checks on the form. We check that both email and
	// password are provided, and also check the format of the email address as
	// a UX-nicety (in case the user makes a typo).
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "This field must be a valid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, http.StatusUnprocessableEntity, "login.tmpl", data)
		return
	}
	// Check whether the credentials are valid. If they're not, add a generic
	// non-field error message and re-display the login page.
	id, err := app.users.Authenticate(form.Email, form.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			form.AddNonFieldError("Email or password is incorrect")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "login.tmpl", data)
		} else {
			app.serverError(w, err)
		}
		return
	}
	// Use the RenewToken() method on the current session to change the session
	// ID. It's good practice to generate a new session ID when the
	// authentication state or privilege levels changes for the user (e.g. login // and logout operations).
	err = app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}
	// Add the ID of the current user to the session, so that they are now
	// 'logged in'.
	app.sessionManager.Put(r.Context(), "authenticatedUserID", id)
	app.flash(r, "You've been logged in successfully!")
	target := app.sessionManager.PopString(r.Context(), "target")
	if target == "" {
		target = "/"
	}
	// Redirect the user to the home page.
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// Log the user out
func (app *application) userLogoutPost(w http.ResponseWriter, r *http.Request) {
	// Remove the authenticatedUserID from the session data.
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")

	// Update the session
	err := app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}

	// Add a flash message to confirm success.
	app.flash(r, "You've been logged out successfully!")
	// Redirect the user to the application home page.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Ping the server
func (app *application) ping(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// View account /account/view
func (app *application) userAccountView(w http.ResponseWriter, r *http.Request) {
	userID := app.authenticatedUserID(r)
	user, err := app.users.Get(userID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		} else {
			app.serverError(w, err)
		}
		return
	}

	count, err := app.bookmarks.Count(userID, models.Filter{})
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.User = user
	data.BookmarkCount = count
	app.render(w, http.StatusOK, "account.tmpl", data)
}

// Change password form
func (app *application) userPasswordShowForm(w http.ResponseWriter, r *http.Request) {
	userID := app.authenticatedUserID(r)
	user, err := app.users.Get(userID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		} else {
			app.serverError(w, err)
		}
		return
	}

	data := app.newTemplateData(r)
	data.User = user
	data.Form = userChangePasswordForm{}
	app.render(w, http.StatusOK, "password.tmpl", data)
}

// Do the password update - POST /account/password/update
func (app *application) userPasswordPost(w http.ResponseWriter, r *http.Request) {
	var form userChangePasswordForm
	err := app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if !app.allowAttempt(r) {
		form.AddNonFieldError(tooManyAttemptsMessage)
		app.tooManyAttempts(w, r, "password.tmpl", form)
		return
	}

	// Validation
	form.CheckField(validator.NotBlank(form.OldPassword), "old_password", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.NewPassword), "new_password", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.ConfirmPassword), "confirm_password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.NewPassword, 8), "new_password", "The password must contain at least 8 characters")
	form.CheckField(validator.MaxBytes(form.NewPassword, models.MaxPasswordBytes), "new_password", passwordTooLong)
	form.CheckField(validator.Equal(form.ConfirmPassword, form.NewPassword), "confirm_password", "Passwords do not match")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, http.StatusUnprocessableEntity, "password.tmpl", data)
		return
	}

	id := app.authenticatedUserID(r)
	err = app.users.PasswordUpdate(id, form.OldPassword, form.NewPassword)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			form.AddNonFieldError("Password is incorrect")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "password.tmpl", data)
		} else {
			app.serverError(w, err)
		}
		return
	}

	app.flash(r, "Password updated successfully!")

	http.Redirect(w, r, "/account/view", http.StatusSeeOther)
}

// Form to choose a theme on the Account page.
type themeForm struct {
	Theme string `form:"theme"`
}

// Save the User's theme - POST /account/theme. With htmx it saves as soon as
// an option is chosen, and the whole page reloads in the new theme.
func (app *application) themePost(w http.ResponseWriter, r *http.Request) {
	var form themeForm
	if err := app.decodePostForm(w, r, &form); err != nil || !models.ValidTheme(form.Theme) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if err := app.users.SetTheme(app.authenticatedUserID(r), form.Theme); err != nil {
		app.serverError(w, err)
		return
	}
	app.flash(r, "Theme saved.")
	if isHTMX(r) {
		w.Header().Set("HX-Refresh", "true")
		return
	}
	http.Redirect(w, r, "/account/view", http.StatusSeeOther)
}
