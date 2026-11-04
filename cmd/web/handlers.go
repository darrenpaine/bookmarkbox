package main

import (
	"errors"
	"fmt"
	"net/http"
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

// Render the Home page: the user's latest bookmarks, or an intro if logged out.
func (app *application) home(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	if data.IsAuthenticated {
		bookmarks, err := app.bookmarks.Latest(app.authenticatedUserID(r), 50)
		if err != nil {
			app.serverError(w, err)
			return
		}
		data.Bookmarks = bookmarks
	}
	app.render(w, http.StatusOK, "home.tmpl", data)
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

// renderBookmarkFormError re-displays a form with a validation error status.
func (app *application) renderBookmarkFormError(w http.ResponseWriter, r *http.Request, page string, form bookmarkForm) {
	data := app.newTemplateData(r)
	data.Form = form
	app.render(w, http.StatusUnprocessableEntity, page, data)
}

// Render the bookmark Create form.
func (app *application) bookmarkCreateShowForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = bookmarkForm{Action: "/bookmarks/new"}
	app.render(w, http.StatusOK, "create.tmpl", data)
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
		if errors.Is(err, models.ErrDuplicateURL) {
			form.AddFieldError("url", "You have already saved this address")
			app.renderBookmarkFormError(w, r, "create.tmpl", form)
		} else {
			app.serverError(w, err)
		}
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Bookmark saved!")
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
	data := app.newTemplateData(r)
	data.Form = bookmarkForm{
		Action:      fmt.Sprintf("/bookmarks/%d/edit", id),
		URL:         bookmark.URL,
		Title:       bookmark.Title,
		Description: bookmark.Description,
		Notes:       bookmark.Notes,
		Tags:        strings.Join(bookmark.Tags, ", "),
		Favourite:   bookmark.Favourite,
		ReadLater:   bookmark.ReadLater,
	}
	app.render(w, http.StatusOK, "edit.tmpl", data)
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
		case errors.Is(err, models.ErrDuplicateURL):
			form.AddFieldError("url", "You have already saved this address")
			app.renderBookmarkFormError(w, r, "edit.tmpl", form)
		default:
			app.serverError(w, err)
		}
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Bookmark updated!")
	http.Redirect(w, r, fmt.Sprintf("/bookmarks/%d", id), http.StatusSeeOther)
}

// Toggle the favourite star, then go back to the bookmark.
func (app *application) bookmarkFavouritePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	err := app.bookmarks.ToggleFavourite(app.authenticatedUserID(r), id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/bookmarks/%d", id), http.StatusSeeOther)
}

// Delete a bookmark, then go to the home page.
func (app *application) bookmarkDeletePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r)
	if !ok {
		app.notFound(w)
		return
	}
	err := app.bookmarks.Delete(app.authenticatedUserID(r), id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Bookmark deleted.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
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

	// Commas split Tags on the bookmark form, so they can't be in a name.
	names := models.NormaliseTags([]string{form.Name})
	problem := ""
	switch {
	case len(names) == 0:
		problem = "A tag name cannot be blank"
	case strings.Contains(form.Name, ","):
		problem = "A tag name cannot contain a comma"
	case !validator.MaxChars(names[0], models.MaxTagNameChars):
		problem = fmt.Sprintf("A tag name must be %d characters or fewer", models.MaxTagNameChars)
	}
	if problem != "" {
		data := app.newTemplateData(r)
		data.Flash = problem
		app.renderTagList(w, r, http.StatusUnprocessableEntity, data)
		return
	}
	name := names[0]
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
		app.sessionManager.Put(r.Context(), "flash", "Tags merged.")
	} else {
		app.sessionManager.Put(r.Context(), "flash", "Tag renamed.")
	}
	http.Redirect(w, r, "/tags", http.StatusSeeOther)
}

// Show a form to create a user
func (app *application) userSignupShowForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = userSignupForm{}
	app.render(w, http.StatusOK, "signup.tmpl", data)
}

// Post method to create a user
func (app *application) userSignupPost(w http.ResponseWriter, r *http.Request) {
	var form userSignupForm
	err := app.decodePostForm(w, r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	// VALIDATION:
	// Note email field is case insensitive in the DB (utf8mb4_unicode_ci)
	form.CheckField(validator.NotBlank(form.Name), "name", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "Invalid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.Password, 8), "password", "The password must contain at least 8 characters")
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
		if errors.Is(err, models.ErrDuplicateEmail) {
			form.AddFieldError("email", "Email address is already in use")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "signup.tmpl", data)
		} else {
			app.serverError(w, err)
		}
		return
	}
	// Otherwise add a confirmation flash message confirming that signup worked.
	app.sessionManager.Put(r.Context(), "flash", "Your signup was successful. Please log in.")
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
	app.sessionManager.Put(r.Context(), "flash", "You've been logged in successfully!")
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
	app.sessionManager.Put(r.Context(), "flash", "You've been logged out successfully!")
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

	data := app.newTemplateData(r)
	data.User = user
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

	// Validation
	form.CheckField(validator.NotBlank(form.OldPassword), "old_password", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.NewPassword), "new_password", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.ConfirmPassword), "confirm_password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.NewPassword, 8), "new_password", "The password must contain at least 8 characters")
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

	app.sessionManager.Put(r.Context(), "flash", "Password updated successfully!")

	http.Redirect(w, r, "/account/view", http.StatusSeeOther)
}
