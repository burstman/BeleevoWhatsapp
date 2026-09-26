package whatsapp

import (
	"fmt"
	"regexp"
	"strings"
)

// TokenKey identifies an auto-filled message variable, shown in the template
// editor as a draggable chip and serialized in the body as {{token}}.
type TokenKey string

const (
	TokenCustomerName  TokenKey = "customer_name"
	TokenCustomerPhone TokenKey = "customer_phone"
	TokenOrderID       TokenKey = "order_id"
	TokenOrderStatus   TokenKey = "order_status"
	TokenTrackingCode  TokenKey = "tracking_code"
	TokenDriverName    TokenKey = "driver_name"
	TokenDriverPhone   TokenKey = "driver_phone"
)

// Event sources a template can be written for. SourceAny is the neutral
// default: the template may be attached to an automation of either source.
const (
	SourceConverty = "converty"
	SourceDelivery = "delivery"
	SourceAny      = "any"
)

// SourceOption is one entry in the editor's "this message is for" picker.
type SourceOption struct {
	Value string
	Label string
	Hint  string
}

// SourceOptions are the source choices offered when creating a template.
func SourceOptions() []SourceOption {
	return []SourceOption{
		{Value: SourceConverty, Label: "Converty orders", Hint: "Fires on order status changes from Converty"},
		{Value: SourceDelivery, Label: "Mes Colis delivery", Hint: "Fires on parcel status changes from Mes Colis"},
		{Value: SourceAny, Label: "Both", Hint: "Usable by either kind of automation"},
	}
}

// SourceLabel renders a stored source for display.
func SourceLabel(source string) string {
	switch source {
	case SourceConverty:
		return "Converty"
	case SourceDelivery:
		return "Mes Colis"
	case SourceAny, "":
		return "Both"
	}
	return source
}

// NormalizeSource maps anything unrecognized to the neutral default, so a bad
// form value can never leave a template unattached.
func NormalizeSource(source string) string {
	switch source {
	case SourceConverty, SourceDelivery, SourceAny:
		return source
	}
	return SourceAny
}

// TokenAllowed reports whether a variable can appear in a template written for
// a given source. Driver details only exist on the delivery side, so a
// Converty template that used them would send an empty slot to the customer.
func TokenAllowed(source string, k TokenKey) bool {
	if source == SourceDelivery || source == SourceAny {
		return true
	}
	return k != TokenDriverName && k != TokenDriverPhone
}

// VariableChip is the palette entry rendered in the template editor. Keep the
// order here: it is the order chips are offered to the author (not the order
// they land in the body — that is decided by dropping).
func VariableChips() []VariableChip {
	return []VariableChip{
		{Value: string(TokenCustomerName), Label: "Customer name", Example: "Karima"},
		{Value: string(TokenCustomerPhone), Label: "Customer phone", Example: "+216 20 000 000"},
		{Value: string(TokenOrderID), Label: "Order id", Example: "ORD-12345"},
		{Value: string(TokenOrderStatus), Label: "Order status", Example: "out for delivery"},
		{Value: string(TokenTrackingCode), Label: "Tracking code", Example: "1234567890113"},
		{Value: string(TokenDriverName), Label: "Driver name", Example: "Ali Mansour"},
		{Value: string(TokenDriverPhone), Label: "Driver phone", Example: "+216 98 111 222"},
	}
}

// Sources lists the event sources a chip may be used in, as a comma-separated
// list. The editor renders it on the chip and hides the ones the chosen source
// cannot fill.
func (c VariableChip) Sources() string {
	if !TokenAllowed(SourceConverty, TokenKey(c.Value)) {
		return "delivery,any"
	}
	return "converty,delivery,any"
}

// VariableChip is one draggable variable the author can drop into the body.
type VariableChip struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Example string `json:"example"`
}

func (k TokenKey) label() string {
	for _, c := range VariableChips() {
		if c.Value == string(k) {
			return c.Label
		}
	}
	return strings.TrimPrefix(string(k), "_")
}

// TemplateVariableValues is the resolved per-send data the map draws from.
type TemplateVariableValues struct {
	CustomerName  string
	CustomerPhone string
	OrderID       string
	StatusLabel   string
	TrackingCode  string
	DriverName    string
	DriverPhone   string
}

// TokenValue resolves one variable to its send-time value. Unknown tokens
// resolve to "", which the sender replaces with a neutral placeholder.
func TokenValue(k TokenKey, v TemplateVariableValues) string {
	switch k {
	case TokenCustomerName:
		return v.CustomerName
	case TokenCustomerPhone:
		return v.CustomerPhone
	case TokenOrderID:
		return v.OrderID
	case TokenOrderStatus:
		return v.StatusLabel
	case TokenTrackingCode:
		return v.TrackingCode
	case TokenDriverName:
		return v.DriverName
	case TokenDriverPhone:
		return v.DriverPhone
	}
	return ""
}

// TokenExample returns a sensible sample value for a token, used by the
// editor's example-value prefill and the message preview.
func TokenExample(k TokenKey) string {
	for _, c := range VariableChips() {
		if c.Value == string(k) {
			return c.Example
		}
	}
	return ""
}

// DefaultTokenForPosition is the legacy positional mapping kept for templates
// written with {{1..N}} before semantic chips existed.
func DefaultTokenForPosition(position int) TokenKey {
	switch position {
	case 1:
		return TokenCustomerName
	case 2:
		return TokenOrderID
	case 3:
		return TokenOrderStatus
	}
	return ""
}

var tokenPattern = regexp.MustCompile(`\{\{([a-zA-Z][a-zA-Z0-9_]*)\}\}`)

// TokenizeTemplateBody converts a semantic body (with {{customer_name}} chips)
// into Meta's positional {{1..N}} body and returns the ordered token keys that
// map to each position. Re-using the same chip re-uses its position. A body
// with no semantic chips is returned unchanged with nil tokens (legacy).
func TokenizeTemplateBody(text string) (positional string, tokens []TokenKey) {
	if text == "" {
		return text, nil
	}

	seen := map[TokenKey]int{}
	var b strings.Builder
	next := 1
	last := 0
	for _, m := range tokenPattern.FindAllStringSubmatchIndex(text, -1) {
		b.WriteString(text[last:m[0]])
		last = m[1]
		key := TokenKey(text[m[2]:m[3]])
		pos, ok := seen[key]
		if !ok {
			pos = next
			next++
			seen[key] = pos
			tokens = append(tokens, key)
		}
		b.WriteString(fmt.Sprintf("{{%d}}", pos))
	}
	b.WriteString(text[last:])

	if len(tokens) == 0 {
		return text, nil
	}
	return b.String(), tokens
}
