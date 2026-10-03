package domain

import "time"

const StatusAvailable = "available"

type Item struct {
	ID                int64     `json:"item_id"`
	Category          string    `json:"category"`
	PublicDescription string    `json:"public_description"`
	FoundPlace        string    `json:"found_place"`
	FoundAt           time.Time `json:"found_at"`
	Status            string    `json:"status"`
}
