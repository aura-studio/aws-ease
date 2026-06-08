package resolver_test

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/aura-studio/aws-ease"
	"github.com/aura-studio/aws-ease/resolver"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		want    awsease.Endpoint
		wantErr bool
	}{
		{
			name:   "http target",
			target: "http://example.com/api?q=1",
			want: awsease.Endpoint{
				Scheme: "http",
				Host:   "example.com",
				Path:   "/api",
				Query:  url.Values{"q": []string{"1"}},
				Raw:    "http://example.com/api?q=1",
			},
		},
		{
			name:   "https target with default path",
			target: "https://example.com",
			want: awsease.Endpoint{
				Scheme: "https",
				Host:   "example.com",
				Path:   "/",
				Query:  url.Values{},
				Raw:    "https://example.com",
			},
		},
		{
			name:   "lambda target",
			target: "lambda://svc/v1/do?x=1",
			want: awsease.Endpoint{
				Scheme: "lambda",
				Host:   "svc",
				Path:   "/v1/do",
				Query:  url.Values{"x": []string{"1"}},
				Raw:    "lambda://svc/v1/do?x=1",
			},
		},
		{
			name:   "sqs target with default path",
			target: "sqs://queue-name",
			want: awsease.Endpoint{
				Scheme: "sqs",
				Host:   "queue-name",
				Path:   "/",
				Query:  url.Values{},
				Raw:    "sqs://queue-name",
			},
		},
		{
			name:    "invalid missing host",
			target:  "lambda:///v1/do",
			wantErr: true,
		},
		{
			name:    "invalid missing scheme",
			target:  "svc/v1/do",
			wantErr: true,
		},
		{
			name:    "invalid unsupported scheme",
			target:  "ftp://example.com/file",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolver.Parse(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) expected error", tt.target)
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.target, err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.target, got, tt.want)
			}
		})
	}
}
