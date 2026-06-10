package tests

import (
	"testing"

	awsease "github.com/aura-studio/aws-ease"
)

func TestResponseOK(t *testing.T) {
	tests := []struct {
		name string
		resp awsease.Response
		want bool
	}{
		{"http 200", awsease.Response{Backend: awsease.BackendHTTP, Status: 200}, true},
		{"http 204", awsease.Response{Backend: awsease.BackendHTTP, Status: 204}, true},
		{"http 404", awsease.Response{Backend: awsease.BackendHTTP, Status: 404}, false},
		{"http 500", awsease.Response{Backend: awsease.BackendHTTP, Status: 500}, false},
		{"http zero status", awsease.Response{Backend: awsease.BackendHTTP, Status: 0}, false},
		{"lambda sync ok", awsease.Response{Backend: awsease.BackendLambda, FuncError: ""}, true},
		{"lambda sync func error", awsease.Response{Backend: awsease.BackendLambda, FuncError: "Unhandled"}, false},
		{"lambda async accepted", awsease.Response{Backend: awsease.BackendLambda, Async: true}, true},
		{"sqs delivered", awsease.Response{Backend: awsease.BackendSQS, MessageID: "abc"}, true},
		{"sqs no message id", awsease.Response{Backend: awsease.BackendSQS, MessageID: ""}, false},
		{"unknown backend", awsease.Response{Backend: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resp.OK(); got != tt.want {
				t.Errorf("OK() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResponseJSONAndString(t *testing.T) {
	r := &awsease.Response{Body: []byte(`{"name":"x","n":2}`)}
	var v struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := r.JSON(&v); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if v.Name != "x" || v.N != 2 {
		t.Fatalf("decoded = %+v", v)
	}
	if r.String() != `{"name":"x","n":2}` {
		t.Fatalf("String() = %q", r.String())
	}
}
