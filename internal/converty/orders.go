package converty

import (
	"context"
	"net/http"
	"strconv"
)

// ordersPageLimit is the Converty page size requested for list walks; the API
// caps it at 200, so a short final page marks the end of the collection.
const ordersPageLimit = 200

// Order is one order of an authenticated store (GET /api/v1/orders).
// Reference is the human order number the Converty dashboard displays; ID is
// the Mongo-style _id the platform previously stored everywhere.
type Order struct {
	ID            string
	Reference     int64
	Barcode       string
	Status        string
	CustomerName  string
	CustomerPhone string
}

type orderDTO struct {
	ID        string `json:"_id"`
	Reference int64  `json:"reference"`
	Barcode   string `json:"barcode"`
	Status    string `json:"status"`
	Customer  struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
	} `json:"customer"`
}

func (d orderDTO) order() Order {
	return Order{
		ID:            d.ID,
		Reference:     d.Reference,
		Barcode:       d.Barcode,
		Status:        d.Status,
		CustomerName:  d.Customer.Name,
		CustomerPhone: d.Customer.Phone,
	}
}

// ListOrders fetches every order in the authenticated store, walking the
// page/limit cursor until a short page proves the end.
func (s *Service) ListOrders(ctx context.Context, accessToken string) ([]Order, error) {
	var orders []Order
	for page := 1; ; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			s.cfg.ConvertyAPIURL+"/api/v1/orders?page="+strconv.Itoa(page)+"&limit="+strconv.Itoa(ordersPageLimit), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		var out struct {
			Data []orderDTO `json:"data"`
		}
		if err := s.doJSON(req, &out); err != nil {
			return nil, err
		}
		for _, d := range out.Data {
			orders = append(orders, d.order())
		}
		if len(out.Data) < ordersPageLimit {
			return orders, nil
		}
	}
}