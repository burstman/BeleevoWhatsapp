package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/anthdm/superkit/validate"

	"whatsappconverty/internal/auth"
	viewauth "whatsappconverty/web/views/auth"
	errortpl "whatsappconverty/web/views/errors"
)

var loginSchema = validate.Schema{
	"email":    validate.Rules(validate.Email),
	"password": validate.Rules(validate.Required),
}

func (a *App) handleLoginGet(k *kit.Kit) error {
	if auth.FromKit(k).LoggedIn {
		return k.Redirect(http.StatusSeeOther, "/dashboard")
	}
	return k.Render(viewauth.LoginPage())
}

func (a *App) handleLoginPost(k *kit.Kit) error {
	var values viewauth.LoginValues
	formErrs, ok := validate.Request(k.Request, &values, loginSchema)
	if !ok {
		return k.Render(viewauth.LoginForm(values, formErrs))
	}

	token, err := a.Auth.Login(k.Request.Context(), values.Email, values.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			formErrs.Add("email", "incorrect email or password")
			formErrs.Add("password", "")
			return k.Render(viewauth.LoginForm(values, formErrs))
		}
		return err
	}

	sess := k.GetSession(auth.SessionCookieName)
	sess.Values["sessionToken"] = token
	if err := sess.Save(k.Request, k.Response); err != nil {
		return err
	}

	redirect := values.Redirect
	if !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		redirect = "/dashboard"
	}
	return k.Redirect(http.StatusSeeOther, redirect)
}

func (a *App) handleLogout(k *kit.Kit) error {
	sess := k.GetSession(auth.SessionCookieName)
	if token, ok := sess.Values["sessionToken"].(string); ok && token != "" {
		_ = a.Auth.Logout(k.Request.Context(), token)
	}
	sess.Options.MaxAge = -1
	_ = sess.Save(k.Request, k.Response)
	return k.Redirect(http.StatusSeeOther, "/login")
}

func (a *App) handleNotFound(k *kit.Kit) error {
	return k.Render(errortpl.NotFoundPage())
}
