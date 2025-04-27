package model

import (
	"time"
)

type Order struct {
	ID          int64        `json:"id"`
	CustomerID  int64        `json:"customer_id"`
	State       OrderState   `json:"state"`
	Weight      float64      `json:"weight"`
	Cost        float64      `json:"cost"`
	PackageType *PackageType `json:"package_type,omitempty"`
	Wrapper     *WrapperType `json:"wrapper,omitempty"`
	DeadlineAt  time.Time    `json:"deadline_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	DeliveredAt *time.Time   `json:"delivered_at,omitempty"`
	ReturnedAt  *time.Time   `json:"returned_at,omitempty"`
}
