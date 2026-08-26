package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

const maxJSONBody = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.FieldError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.FieldError{Field: "body", Message: "must contain one JSON object"}
	}
	return nil
}

func parseDuration(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, domain.FieldError{Field: "duration", Message: "must be a positive Go duration"}
	}
	return duration, nil
}
