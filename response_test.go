package awsease

import "testing"

func TestResponseOK(t *testing.T) {
	tests := []struct {
		name string
		resp Response
		want bool
	}{
		{"http 200", Response{Backend: BackendHTTP, Status: 200}, true},
		{"http 204", Response{Backend: BackendHTTP, Status: 204}, true},
		{"http 404", Response{Backend: BackendHTTP, Status: 404}, false},
		{"http 500", Response{Backend: BackendHTTP, Status: 500}, false},
		{"http zero status", Response{Backend: BackendHTTP, Status: 0}, false},
		{"lambda sync ok", Response{Backend: BackendLambda, FuncError: ""}, true},
		{"lambda sync func error", Response{Backend: BackendLambda, FuncError: "Unhandled"}, false},
		{"lambda async accepted", Response{Backend: BackendLambda, Async: true}, true},
		{"sqs delivered", Response{Backend: BackendSQS, MessageID: "abc"}, true},
		{"sqs no message id", Response{Backend: BackendSQS, MessageID: ""}, false},
		{"unknown backend", Response{Backend: ""}, false},
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
	r := &Response{Body: []byte(`{"name":"x","n":2}`)}
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
