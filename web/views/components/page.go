package components

import (
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
}

func (p Page) ActiveClass(section string) string {
	if p.Active == section {
		return "bg-indigo-600 text-white"
	}
	return "text-slate-300 hover:bg-slate-800 hover:text-white"
}
