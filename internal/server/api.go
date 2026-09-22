package server

import (
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/whatsapp"
)

// apiError is the stable public error envelope. Codes match the whatsapp
// package SendRejection codes so clients can act on them.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON writes a JSON payload with the given status.
func writeJSON(k *kit.Kit, status int, payload any) error {
	return k.JSON(status, payload)
}

// writeAPIError maps internal errors into the safe public envelope. Internal
// details are returned only when they are already safe (SendRejection);
// everything else becomes a generic 500 and is logged by the caller.
func (a *App) writeAPIError(k *kit.Kit, status int, err error) error {
	var rej *whatsapp.SendRejection
	if errors.As(err, &rej) {
		status = apiStatusForCode(rej.Code)
		return writeJSON(k, status, apiError{Code: rej.Code, Message: rej.Reason})
	}
	a.Log.Error("api error", "path", k.Request.URL.Path, "error", err.Error())
	return writeJSON(k, http.StatusInternalServerError, apiError{Code: "internal_error", Message: "something went wrong"})
}

// apiStatusForCode maps the stable pre-send rejection codes to HTTP statuses.
func apiStatusForCode(code string) int {
	switch code {
	case whatsapp.ErrCodeOptInRequired,
		whatsapp.ErrCodeOptInRevoked,
		whatsapp.ErrCodeServiceDisabled,
		whatsapp.ErrCodeTermsNotAccepted,
		whatsapp.ErrCodeCustomerNotOwned,
		whatsapp.ErrCodeTemplateNotFound,
		whatsapp.ErrCodeTemplateNotApproved,
		whatsapp.ErrCodeTemplateUnintendedPurpose,
		whatsapp.ErrCodeTemplateVariableInvalid,
		whatsapp.ErrCodeDuplicateSend:
		return http.StatusUnprocessableEntity
	case whatsapp.ErrCodeMerchantNotAuthorized:
		return http.StatusForbidden
	case whatsapp.ErrCodeRateLimited:
		return http.StatusTooManyRequests
	case whatsapp.ErrCodeMetaAPIError:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
