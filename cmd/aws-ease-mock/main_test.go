package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEchoHandler(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/lambda/svc/do?x=1", bytes.NewBufferString("payload"))
	w := httptest.NewRecorder()

	echo("lambda")(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Route != "lambda" || resp.Path != "/lambda/svc/do" || resp.Query["x"][0] != "1" || resp.Payload != "payload" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
