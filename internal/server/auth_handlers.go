package server

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/anthdm/superkit/validate"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/i18n"
	viewauth "whatsappconverty/web/views/auth"
	errortpl "whatsappconverty/web/views/errors"
)

// emailRule accepts any syntactically valid mailbox instead of superkit's
// validate.Email, whose regex caps the TLD at four characters and would reject
// the operator default "admin@bleevoo.local". The message is resolved per
// request so it can be shown in the operator's language.
func emailRule(dict *i18n.Dict) validate.RuleSet {
	return validate.RuleSet{
		Name: "email",
		MessageFunc: func(_ validate.RuleSet) string {
			return dict.T("auth.emailInvalid")
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
}

func loginSchema(dict *i18n.Dict) validate.Schema {
	return validate.Schema{
		"email":    validate.Rules(emailRule(dict)),
		"password": validate.Rules(validate.Required),
	}
}

func (a *App) handleLoginGet(k *kit.Kit) error {
	if auth.FromKit(k).LoggedIn {
		return k.Redirect(http.StatusSeeOther, "/dashboard")
	}
	return k.Render(viewauth.LoginPage(i18n.New(a.publicLang(k))))
}

func (a *App) handleLoginPost(k *kit.Kit) error {
	dict := i18n.New(a.publicLang(k))
	var values viewauth.LoginValues
	formErrs, ok := validate.Request(k.Request, &values, loginSchema(dict))
	if !ok {
		return k.Render(viewauth.LoginForm(values, formErrs, dict))
	}

	token, err := a.Auth.Login(k.Request.Context(), values.Email, values.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			formErrs.Add("email", dict.T("auth.badCredentials"))
			formErrs.Add("password", "")
			return k.Render(viewauth.LoginForm(values, formErrs, dict))
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

// handleNotFound renders the 404 page. The status has to be set explicitly:
// Render only streams the component to the ResponseWriter, so without this a
// missing page answers 200. Link checkers (including Meta's) then treat a dead
// link as a working page.
func (a *App) handleNotFound(k *kit.Kit) error {
	k.Response.WriteHeader(http.StatusNotFound)
	return k.Render(errortpl.NotFoundPage(i18n.New(a.publicLang(k))))
}

// handleLanguagePost switches the operator's interface language. The choice is
// saved on the account (so it follows them to any device), mirrored into a
// durable cookie (so the public login/landing pages follow too), then bounced
// back to the page they were on.
func (a *App) handleLanguagePost(k *kit.Kit) error {
	principal := auth.FromKit(k)
	lang := i18n.Parse(k.Request.FormValue("lang"))
	if principal.LoggedIn {
		if err := a.Auth.SetLang(k.Request.Context(), principal.User.ID, lang); err != nil {
			return err
		}
	}
	http.SetCookie(k.Response, &http.Cookie{
		Name:     "lang",
		Value:    lang.String(),
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
	})

	ref := k.Request.Referer()
	if !strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "//") {
		ref = "/dashboard"
	}
	return k.Redirect(http.StatusSeeOther, ref)
}
