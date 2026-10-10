package server

import (
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/i18n"
	viewserrors "whatsappconverty/web/views/errors"
)

// ErrorHandler is registered with kit.UseErrorHandler. It turns any error that
// bubbles up from a handler into an HTTP response without leaking internals.
func (a *App) ErrorHandler(k *kit.Kit, err error) {
	logger := a.Log.With("request_id", chimiddleware.GetReqID(k.Request.Context()))
	logger.Error("request failed", "error", err.Error())

	var status int
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		status = http.StatusUnauthorized
	default:
		status = http.StatusInternalServerError
	}

	if k.Request.Header.Get("HX-Request") == "true" {
		k.Response.Header().Set("HX-Retarget", "#toast")
		_ = k.Render(viewserrors.ErrorToast(i18n.New(a.publicLang(k)).T(errorTitleKey(status))))
		return
	}

	dict := i18n.New(a.publicLang(k))
	_ = k.Render(viewserrors.ErrorPage(status, dict.T(errorTitleKey(status)), dict.T("error.genericBody"), dict))
}

// errorTitleKey maps an HTTP status to a translated error heading. The status
// is still shown as a big number above it, so the heading can stay a clean
// sentence instead of the technical english status text.
func errorTitleKey(status int) string {
	if status == http.StatusUnauthorized {
		return "error.unauthorizedTitle"
	}
	return "error.serverErrorTitle"
}
