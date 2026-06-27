package ws

import "encoding/json"

// Message is the envelope exchanged over the socket. Keep it small and stable;
// clients depend on the JSON shape.
type Message struct {
	Type string `json:"type"`           // e.g. "message", "system"
	From string `json:"from,omitempty"` // sender user id
	Body string `json:"body,omitempty"` // text payload
}

func encode(m Message) []byte {
	b, _ := json.Marshal(m)
	return b
}
