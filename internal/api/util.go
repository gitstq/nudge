package api

import (
	"encoding/json"
	"net/http"
)

// jsonDecoder returns a tolerant decoder for inbound publish payloads:
// publishers are not rejected just because they sent an extra field.
func jsonDecoder(r *http.Request) *json.Decoder {
	dec := json.NewDecoder(r.Body)
	return dec
}
