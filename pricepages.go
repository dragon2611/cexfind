package cexfind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	priceCacheTTL    = 2 * time.Minute
	maxPriceSearches = 8
)

// A bounded snapshot per search. The mutex on CexFind also coalesces simultaneous
// requests for the same page. Store distances are computed on copies afterwards.
type priceSearch struct {
	expires time.Time
	streams []priceStream
	boxes   map[string]Box
}

type priceStream struct {
	query    string
	page     int
	done     bool
	boundary decimal.Decimal
}

func cleanQueries(queries []string) ([]string, error) {
	out := make([]string, 0, len(queries))
	seen := make(map[string]bool)
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" || !validQuery(query) {
			return nil, fmt.Errorf("query %q is empty, too long or not valid", query)
		}
		if !seen[query] {
			out = append(out, query)
			seen[query] = true
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one query is required")
	}
	if len(out) > maxSearchQueries {
		return nil, fmt.Errorf("at most %d distinct queries are allowed per search", maxSearchQueries)
	}
	return out, nil
}

func priceSearchKey(queries []string, strict bool, price PriceRange, order string) string {
	sorted := slices.Clone(queries)
	slices.Sort(sorted)
	key, _ := json.Marshal(struct {
		URL             string
		Queries         []string
		Strict          bool
		Min, Max, Order string
	}{URL, sorted, strict, priceBound(price.Min), priceBound(price.Max), order})
	return string(key)
}

func priceBound(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func (cex *CexFind) mergedPricePage(ctx context.Context, queries []string, strict bool, page int, price PriceRange, order string) ([]Box, bool, error) {
	cex.searchMu.Lock()
	defer cex.searchMu.Unlock()
	now := time.Now()
	if cex.priceSearches == nil {
		cex.priceSearches = make(map[string]*priceSearch)
	}
	for key, search := range cex.priceSearches {
		if !now.Before(search.expires) {
			delete(cex.priceSearches, key)
		}
	}
	key := priceSearchKey(queries, strict, price, order)
	search := cex.priceSearches[key]
	if search == nil {
		if len(cex.priceSearches) >= maxPriceSearches {
			var oldest string
			for k, s := range cex.priceSearches {
				if oldest == "" || s.expires.Before(cex.priceSearches[oldest].expires) {
					oldest = k
				}
			}
			delete(cex.priceSearches, oldest)
		}
		search = &priceSearch{expires: now.Add(priceCacheTTL), boxes: make(map[string]Box)}
		for _, query := range queries {
			search.streams = append(search.streams, priceStream{query: query})
		}
		cex.priceSearches[key] = search
	}
	target := (page + 1) * pageSize
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		boxes := make([]Box, 0, len(search.boxes))
		for _, box := range search.boxes {
			boxes = append(boxes, box)
		}
		SortBoxes(boxes, order)
		// Only expand streams whose unseen prices could precede this page's end.
		// Equality matters: equal prices can span batches, with model/ID tie breaks.
		next := -1
		for i := range search.streams {
			stream := &search.streams[i]
			if stream.done {
				continue
			}
			if stream.page == 0 {
				next = i
				break
			}
			if len(boxes) >= target {
				cmp := stream.boundary.Compare(boxes[target-1].Price)
				if (order == SortPrice && cmp > 0) || (order == SortPriceDesc && cmp < 0) {
					continue
				}
			}
			if next < 0 || earlierPrice(stream.boundary, search.streams[next].boundary, order) {
				next = i
			}
		}
		if next < 0 {
			start := min(page*pageSize, len(boxes))
			end := min(target, len(boxes))
			more := end < len(boxes)
			for _, stream := range search.streams {
				more = more || !stream.done
			}
			return boxes[start:end], more, nil
		}
		stream := &search.streams[next]
		response, err := fetchQuery(ctx, cex.client, stream.query, stream.page, pageSize, price, order)
		if err != nil && !errors.Is(err, ErrNoResultsFound) {
			// Keep successful batches but never serve a potentially misordered page.
			return nil, false, fmt.Errorf("%q: %w", stream.query, err)
		}
		stream.page++
		if len(response.Results) == 0 || len(response.Results[0].Hits) == 0 {
			stream.done = true
			continue
		}
		result := response.Results[0]
		stream.boundary = result.Hits[len(result.Hits)-1].Price
		stream.done = stream.page >= result.NbPages || stream.page*pageSize >= upstreamHitLimit
		for _, hit := range result.Hits {
			box := Box{Model: extractModelType(hit.BoxName), Name: hit.BoxName, Category: hit.Category, ID: hit.BoxID,
				Price: hit.Price, PriceCash: hit.PriceCash, PriceExchange: hit.PriceExchange, storeNames: storeSimplifier(hit.Stores)}
			if strict && !box.inQuery(queries) {
				continue
			}
			if _, exists := search.boxes[box.ID]; !exists {
				search.boxes[box.ID] = box
			}
		}
	}
}

func earlierPrice(a, b decimal.Decimal, order string) bool {
	if order == SortPriceDesc {
		return a.GreaterThan(b)
	}
	return a.LessThan(b)
}
