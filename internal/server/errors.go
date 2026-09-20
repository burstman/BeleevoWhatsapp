package server

import (
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"whatsappconverty/internal/auth"
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
		_ = k.Render(viewserrors.ErrorToast(http.StatusText(status)))
		return
	}

	_ = k.Render(viewserrors.ErrorPage(status, http.StatusText(status), "Something went wrong. Please try again."))
}
