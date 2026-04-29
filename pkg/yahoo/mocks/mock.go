// Package mocks provides test doubles for the yahoo package.
package mocks

import (
	"context"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

// Quoter is the interface satisfied by yahoo.Client.
type Quoter interface {
	GetQuote(ctx context.Context, ticker string) (*yahoo.Quote, error)
}

// MockQuoter is a test double for Quoter.
type MockQuoter struct {
	QuoteFn func(ctx context.Context, ticker string) (*yahoo.Quote, error)
}

// GetQuote delegates to QuoteFn.
func (m *MockQuoter) GetQuote(ctx context.Context, ticker string) (*yahoo.Quote, error) {
	return m.QuoteFn(ctx, ticker)
}
