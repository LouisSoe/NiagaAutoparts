package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Category struct {
	ID          int64          `db:"id" json:"id"`
	Name        string         `db:"name" json:"name"`
	Slug        string         `db:"slug" json:"slug"`
	Description sql.NullString `db:"description" json:"description"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`
}

func (c Category) MarshalJSON() ([]byte, error) {
	type Alias Category
	var desc *string
	if c.Description.Valid {
		desc = &c.Description.String
	}
	return json.Marshal(&struct {
		Alias
		Description *string `json:"description"`
	}{
		Alias:       Alias(c),
		Description: desc,
	})
}

func (c *Category) UnmarshalJSON(data []byte) error {
	type Alias Category
	aux := &struct {
		*Alias
		Description *string `json:"description"`
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Description != nil {
		c.Description.String = *aux.Description
		c.Description.Valid = true
	} else {
		c.Description.Valid = false
	}
	return nil
}

