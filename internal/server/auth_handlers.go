package server

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/anthdm/superkit/validate"

	"whatsappconverty/internal/auth"
	viewauth "whatsappconverty/web/views/auth"
	errortpl "whatsappconverty/web/views/errors"
)

// emailRule accepts any syntactically valid mailbox instead of superkit's
// validate.Email, whose regex caps the TLD at four characters and would reject
// the operator default "admin@bleevoo.local".
var emailRule = validate.RuleSet{
	Name: "email",
	MessageFunc: func(_ validate.RuleSet) string {
		return "is not a valid email address"
	},
	ValidateFunc: func(set validate.RuleSet) bool {
		email, ok := set.FieldValue.(string)
		if !ok {
			return false
		}
		addr, err := mail.ParseAddress(email)
		return err == nil && addr.Address == email
	},
}

var loginSchema = validate.Schema{
	"email":    validate.Rules(emailRule),
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
