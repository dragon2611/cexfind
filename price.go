package cexfind

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// PriceRange limits the selling price in pounds. Nil bounds are unrestricted.
// Both bounds are inclusive.
type PriceRange struct {
	Min *decimal.Decimal
	Max *decimal.Decimal
}

// ParsePriceRange accepts optional non-negative pound amounts.
func ParsePriceRange(min, max string) (PriceRange, error) {
	var price PriceRange
	for _, bound := range []struct {
		name   string
		value  string
		target **decimal.Decimal
	}{
		{"min price", min, &price.Min},
		{"max price", max, &price.Max},
	} {
		value := strings.TrimSpace(bound.value)
		if value == "" {
			continue
		}
		amount, err := decimal.NewFromString(value)
		if err != nil || amount.IsNegative() {
			return PriceRange{}, fmt.Errorf("%s must be a non-negative number", bound.name)
		}
		*bound.target = &amount
	}
	if price.Min != nil && price.Max != nil && price.Min.GreaterThan(*price.Max) {
		return PriceRange{}, fmt.Errorf("min price must not exceed max price")
	}
	return price, nil
}

func (p PriceRange) validate() error {
	if p.Min != nil && p.Min.IsNegative() {
		return fmt.Errorf("min price must be non-negative")
	}
	if p.Max != nil && p.Max.IsNegative() {
		return fmt.Errorf("max price must be non-negative")
	}
	if p.Min != nil && p.Max != nil && p.Min.GreaterThan(*p.Max) {
		return fmt.Errorf("min price must not exceed max price")
	}
	return nil
}
