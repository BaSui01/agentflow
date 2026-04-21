package usecase

import "encoding/json"

type CreateToolRegistrationInput struct {
	Name        string
	Description string
	Target      string
	Parameters  json.RawMessage
	Enabled     *bool
}

type UpdateToolRegistrationInput struct {
	Name        *string
	Description *string
	Target      *string
	Parameters  *json.RawMessage
	Enabled     *bool
}
