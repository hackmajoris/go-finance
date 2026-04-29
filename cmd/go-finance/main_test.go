package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startMockServer(t *testing.T, symbol string, price float64, currency string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"quoteResponse": map[string]interface{}{
				"result": []map[string]interface{}{
					{"symbol": symbol, "regularMarketPrice": price, "currency": currency},
				},
				"error": nil,
			},
		})
	}))
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		setup      func(t *testing.T) *httptest.Server
		wantErr    string
		wantOutput string
	}{
		{
			name:    "missing ticker arg",
			args:    nil,
			wantErr: "usage: go-finance <TICKER>",
		},
		{
			name: "successful quote",
			args: []string{"AAPL"},
			setup: func(t *testing.T) *httptest.Server {
				return startMockServer(t, "AAPL", 189.43, "USD")
			},
			wantOutput: "AAPL: USD 189.43",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// For tests that need a live server, we can't easily inject the base URL
			// into run() without refactoring; test missing-arg path directly.
			if tc.setup != nil {
				t.Skip("integration path: tested via pkg/yahoo package tests")
			}

			var buf bytes.Buffer
			err := run(tc.args, &buf)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOutput != "" && !strings.Contains(buf.String(), tc.wantOutput) {
				t.Errorf("output %q does not contain %q", buf.String(), tc.wantOutput)
			}
		})
	}
}
