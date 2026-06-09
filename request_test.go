package awsease

import (
	"net/http"
	"testing"
)

func TestHTTPMethod(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{"empty body defaults to GET", Request{}, http.MethodGet},
		{"body defaults to POST", Request{Body: []byte("x")}, http.MethodPost},
		{"explicit method wins over GET default", Request{Method: http.MethodDelete}, http.MethodDelete},
		{"explicit method wins over POST default", Request{Method: http.MethodPut, Body: []byte("x")}, http.MethodPut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.httpMethod(); got != tt.want {
				t.Errorf("httpMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}
