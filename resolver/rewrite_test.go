package resolver_test

import (
	"testing"

	"github.com/aura-studio/aws-ease/resolver"
)

func TestResolverRewrite(t *testing.T) {
	t.Parallel()

	r := resolver.New(
		resolver.Rewrite("lambda://*", "http://localhost:8080/{host}{path}?{query}"),
	)

	got, err := r.Parse("lambda://svc/v1/do?x=1&y=2")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got.Scheme != "http" {
		t.Fatalf("expected scheme http, got %q", got.Scheme)
	}
	if got.Host != "localhost:8080" {
		t.Fatalf("expected host localhost:8080, got %q", got.Host)
	}
	if got.Path != "/svc/v1/do" {
		t.Fatalf("expected path /svc/v1/do, got %q", got.Path)
	}
	if got.Query.Get("x") != "1" || got.Query.Get("y") != "2" {
		t.Fatalf("expected query x=1,y=2, got %#v", got.Query)
	}
}

func TestResolverRewritePassthroughWithoutMatch(t *testing.T) {
	t.Parallel()

	r := resolver.New(resolver.Rewrite("lambda://*", "http://localhost:8080/{host}{path}"))

	got, err := r.Parse("https://example.com/v1/do")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Scheme != "https" || got.Host != "example.com" || got.Path != "/v1/do" {
		t.Fatalf("unexpected endpoint: %#v", got)
	}
}
