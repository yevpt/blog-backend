package providers

import (
	"encoding/json"
	"net/http"
)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type jsonNumber = json.Number

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
