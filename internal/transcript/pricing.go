package transcript

import "strings"

// price is USD per million tokens.
type price struct{ in, out, read float64 }

// Anthropic list prices, 2026-09. Ordered longest prefix first.
// Cache writes are billed at 1.25x input (5 minute) and 2x input (1 hour).
var prices = []struct {
	prefix string
	p      price
}{
	{"claude-fable-5-1", price{10, 50, 0.25}},
	{"claude-mythos-5-1", price{10, 50, 0.25}},
	{"claude-fable-5", price{10, 50, 1}},
	{"claude-mythos", price{10, 50, 1}},
	{"claude-opus-5-5", price{4, 20, 0.20}},
	{"claude-opus-5", price{5, 25, 0.50}},
	{"claude-opus-4", price{5, 25, 0.50}},
	{"claude-sonnet-5", price{2, 10, 0.20}},
	{"claude-sonnet-4", price{3, 15, 0.30}},
	{"claude-haiku-4", price{1, 5, 0.10}},
	{"claude-", price{2, 10, 0.20}}, // unknown Claude model: Sonnet 5.5 pricing
}

// Cost estimates the USD cost of u on model at API list prices. ok is false
// for models tripline has no prices for (non-Anthropic models).
func Cost(model string, u Usage) (usd float64, ok bool) {
	for _, e := range prices {
		if strings.HasPrefix(model, e.prefix) {
			p := e.p
			return (float64(u.Input)*p.in +
				float64(u.Output)*p.out +
				float64(u.CacheWrite5m)*p.in*1.25 +
				float64(u.CacheWrite1h)*p.in*2 +
				float64(u.CacheRead)*p.read) / 1e6, true
		}
	}
	return 0, u == Usage{}
}

// SessionCost sums the estimated cost of every usage record in events.
// priced is false when some tokens came from a model with no known prices.
func SessionCost(events []Event) (usd float64, priced bool) {
	priced = true
	for _, e := range events {
		if e.Usage != nil {
			c, ok := Cost(e.Model, *e.Usage)
			usd += c
			priced = priced && ok
		}
	}
	return usd, priced
}
