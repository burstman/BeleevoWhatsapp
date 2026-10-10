package viewsdashboard

import (
	"encoding/json"

	"whatsappconverty/internal/whatsapp"
)

// templateEditPayload is the JSON the templates page hands to the editor when
// the operator clicks Edit on a negative template. It is embedded in the
// button's data-edit attribute and parsed by window.startTemplateEdit.
type templateEditPayload struct {
	Action   string            `json:"action"`
	Name     string            `json:"name"`
	Language string            `json:"language"`
	Source   string            `json:"source"`
	Body     string            `json:"body"`
	Examples map[string]string `json:"examples"`
	Deadline int64             `json:"deadline"` // unix millis when the grace window closes
}

// templateEditJSON serializes the edit payload for one template row.
func templateEditJSON(t whatsapp.MerchantTemplate, deadlineMS int64) string {
	b, err := json.Marshal(templateEditPayload{
		Action:   "/templates/" + t.ID.String() + "/update",
		Name:     t.Name,
		Language: t.Language,
		Source:   t.Source,
		Body:     whatsapp.SemanticBodyForEdit(t),
		Examples: whatsapp.TemplateExamples(t),
		Deadline: deadlineMS,
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}
