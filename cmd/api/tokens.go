package main

import (
	"errors"
	"greenlight/internal/data"
	"greenlight/internal/validator"
	"net/http"
	"time"
)

func (app *application) createAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestError(w, r, err)
		return
	}

	v := validator.New()

	data.ValidateEmail(v, input.Email)
	data.ValidatePasswordPlaintext(v, input.Password)

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.invalidCredentialResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	match, err := user.Password.Matches(input.Password)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if !match {
		app.invalidCredentialResponse(w, r)
		return
	}

	token, err := app.models.Tokens.New(user.ID, 3*24*time.Hour, data.ScopeAuthentication)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{"authentication_token": token}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}

}

func (app *application) createActivationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestError(w, r, err)
		return
	}

	v := validator.New()

	if data.ValidateEmail(v, input.Email); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			v.AddError("email", "no matching email address found")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if user.Activated {
		v.AddError("email", "user has already been activated")
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	token, err := app.models.Tokens.New(user.ID, 3*24*time.Hour, data.ScopeActivation)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		data := map[string]string{
			"activationToken": token.Plaintext,
		}

		err = app.mailer.Send(user.Email, "token_activation.tmpl", data)
		if err != nil {
			app.logger.PrintError(err, nil)
		}
	})

	envelope := envelope{"message": "an email will be sent to you containing the activation instructions"}
	err = app.writeJSON(w, http.StatusOK, envelope, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}