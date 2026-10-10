package components

import (
	"whatsappconverty/internal/i18n"
	"whatsappconverty/internal/shops"
)

// Page carries the common data rendered on every dashboard page.
type Page struct {
	Title             string
	Active            string
	UserName          string
	Shops             []shops.Shop
	StoreConnected    bool // at least one Converty store is connected
	WhatsAppConnected bool // the operator's WhatsApp number is connected
	UnreadInbox       int  // total unread inbox conversations (nav badge)
	I18N              *i18n.Dict
}

// T translates a key in the page language. A template rendered without an
// injected table (snapshot tests, unset handler values) still falls back to
// English instead of leaking a bare key.
func (p Page) T(key string) string {
	if p.I18N == nil {
		return i18n.New(i18n.En).T(key)
	}
	return p.I18N.T(key)
}

// Tf translates and formats a key with count-style arguments.
func (p Page) Tf(key string, args ...any) string {
	if p.I18N == nil {
		return i18n.New(i18n.En).Tf(key, args...)
	}
	return p.I18N.Tf(key, args...)
}

func (p Page) ActiveClass(section string) string {
	if p.Active == section {
		return "bg-indigo-600 text-white"
	}
	return "text-slate-300 hover:bg-slate-800 hover:text-white"
}
