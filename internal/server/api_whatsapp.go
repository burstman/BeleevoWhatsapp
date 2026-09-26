package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/whatsapp"
)

// POST /api/whatsapp/customers — register a customer for the merchant.
func (a *App) handleAPICreateCustomer(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	var in struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(k.Request.Body).Decode(&in); err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, errors.New("invalid request body"))
	}
	if in.Phone == "" {
		return a.writeAPIError(k, http.StatusBadRequest, whatsapp.NewSendRejection(whatsapp.ErrCodeTemplateVariableInvalid, "phone is required"))
	}

	id, err := a.WhatsApp.UpsertCustomer(k.Request.Context(), shopID, in.Name, in.Phone)
	if err != nil {
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusCreated, map[string]string{"id": id.String()})
}

// GET /api/whatsapp/customers — list the merchant's customers.
func (a *App) handleAPICustomers(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID
	customers, err := a.WhatsApp.Customers(k.Request.Context(), shopID)
	if err != nil {
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusOK, customers)
}

// POST /api/whatsapp/consent — record a customer opt-in for the merchant.
func (a *App) handleAPIGrantConsent(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	var in struct {
		CustomerID string `json:"customer_id"`
		Category   string `json:"category"`
		Source     string `json:"source"`
		Evidence   string `json:"evidence"`
	}
	if err := json.NewDecoder(k.Request.Body).Decode(&in); err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, errors.New("invalid request body"))
	}
	cid, err := uuid.Parse(in.CustomerID)
	if err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, whatsapp.NewSendRejection(whatsapp.ErrCodeCustomerNotOwned, "customer_id is required"))
	}

	if err := a.WhatsApp.GrantConsent(k.Request.Context(), shopID, cid, in.Category, in.Source, in.Evidence); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a.writeAPIError(k, http.StatusUnprocessableEntity, whatsapp.NewSendRejection(whatsapp.ErrCodeCustomerNotOwned, "customer not found for this merchant"))
		}
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusCreated, map[string]string{"status": "opt_in"})
}

// POST /api/whatsapp/revoke — revoke a customer's opt-in.
func (a *App) handleAPIRevokeConsent(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	var in struct {
		CustomerID string `json:"customer_id"`
	}
	if err := json.NewDecoder(k.Request.Body).Decode(&in); err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, errors.New("invalid request body"))
	}
	cid, err := uuid.Parse(in.CustomerID)
	if err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, whatsapp.NewSendRejection(whatsapp.ErrCodeCustomerNotOwned, "customer_id is required"))
	}

	if err := a.WhatsApp.RevokeConsent(k.Request.Context(), shopID, cid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a.writeAPIError(k, http.StatusUnprocessableEntity, whatsapp.NewSendRejection(whatsapp.ErrCodeCustomerNotOwned, "customer not found for this merchant"))
		}
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusOK, map[string]string{"status": "revoked"})
}

// POST /api/whatsapp/templates — submit a new template for Meta review.
func (a *App) handleAPICreateTemplate(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	var in struct {
		Name       string          `json:"name"`
		Language   string          `json:"language"`
		Category   string          `json:"category"`
		Components json.RawMessage `json:"components"`
	}
	if err := json.NewDecoder(k.Request.Body).Decode(&in); err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, errors.New("invalid request body"))
	}
	if in.Name == "" || in.Language == "" || in.Category == "" || len(in.Components) == 0 {
		return a.writeAPIError(k, http.StatusBadRequest,
			whatsapp.NewSendRejection(whatsapp.ErrCodeTemplateVariableInvalid, "name, language, category and components are required"))
	}

	tmpl, err := a.WhatsApp.CreateTemplate(k.Request.Context(), shopID, whatsapp.TemplateDraft{
		Name:       in.Name,
		Language:   in.Language,
		Category:   in.Category,
		Components: in.Components,
	})
	if err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			// Submission rejected by Meta is still a useful 201 with the stored
			// row so the merchant sees approval_status=rejected + reason.
			return writeJSON(k, http.StatusCreated, tmpl)
		}
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusCreated, tmpl)
}

// GET /api/whatsapp/templates — list the merchant's templates.
func (a *App) handleAPITemplates(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID
	templates, err := a.WhatsApp.Templates(k.Request.Context(), shopID)
	if err != nil {
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusOK, templates)
}

// POST /api/whatsapp/messages — send a message through the shared WABA.
func (a *App) handleAPISendMessage(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	var in struct {
		CustomerID      string            `json:"customer_id"`
		TemplateID      string            `json:"template_id"`
		ConvertyOrderID string            `json:"converty_order_id"`
		Purpose         string            `json:"purpose"`
		Variables       map[string]string `json:"variables"`
		IdempotencyKey  string            `json:"idempotency_key"`
	}
	if err := json.NewDecoder(k.Request.Body).Decode(&in); err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, errors.New("invalid request body"))
	}
	cid, err := uuid.Parse(in.CustomerID)
	if err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, whatsapp.NewSendRejection(whatsapp.ErrCodeCustomerNotOwned, "customer_id is required"))
	}
	tid, err := uuid.Parse(in.TemplateID)
	if err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, whatsapp.NewSendRejection(whatsapp.ErrCodeTemplateNotFound, "template_id is required"))
	}

	res, err := a.WhatsApp.SendTemplateMessage(k.Request.Context(), whatsapp.SendRequest{
		ShopID:          shopID,
		CustomerID:      cid,
		TemplateID:      tid,
		ConvertyOrderID: in.ConvertyOrderID,
		Purpose:         in.Purpose,
		Variables:       in.Variables,
		IdempotencyKey:  in.IdempotencyKey,
	})
	if err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			return a.writeAPIError(k, 0, rej)
		}
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusCreated, res)
}

// GET /api/whatsapp/messages — list the merchant's messages.
func (a *App) handleAPIMessages(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID
	messages, err := a.WhatsApp.Messages(k.Request.Context(), shopID)
	if err != nil {
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusOK, messages)
}

// GET /api/whatsapp/messages/{id} — one message.
func (a *App) handleAPIMessage(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return a.writeAPIError(k, http.StatusBadRequest, errors.New("invalid message id"))
	}
	m, err := a.WhatsApp.Message(k.Request.Context(), shopID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a.writeAPIError(k, http.StatusNotFound, whatsapp.NewSendRejection(whatsapp.ErrCodeTemplateNotFound, "message not found"))
		}
		a.writeAPIError(k, http.StatusInternalServerError, err)
		return nil
	}
	return writeJSON(k, http.StatusOK, m)
}
