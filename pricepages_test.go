package cexfind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestMain(m *testing.M) {
	// Production remains 500 ms. Unit tests only contact local mock servers;
	// timing and cooldown behavior are tested separately with dedicated gates.
	sharedSearchGate = newSearchGate(time.Millisecond)
	os.Exit(m.Run())
}

type priceBackend struct {
	mu        sync.Mutex
	calls     map[string][]int
	records   map[string][]Box
	failQuery string
}

func mockPriceBackend(t *testing.T, records map[string][]Box) *priceBackend {
	t.Helper()
	b := &priceBackend{calls: make(map[string][]int), records: records}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Requests []struct {
				IndexName string
				Params    string
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Requests) != 1 {
			t.Errorf("expected one query per request")
			return
		}
		params, err := url.ParseQuery(body.Requests[0].Params)
		if err != nil {
			t.Error(err)
			return
		}
		query := params.Get("query")
		var page int
		fmt.Sscan(params.Get("page"), &page)
		if params.Get("hitsPerPage") != "50" {
			t.Errorf("oversized batch: %v", params)
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.calls[query] = append(b.calls[query], page)
		if b.failQuery == query {
			http.Error(w, "temporary failure", 503)
			return
		}
		records := slices.Clone(b.records[query])
		order := SortPrice
		if body.Requests[0].IndexName == searchIndexPriceDesc {
			order = SortPriceDesc
		}
		// Keep upstream tie ordering deliberately different from local model/ID order.
		slices.SortStableFunc(records, func(a, b Box) int {
			cmp := a.Price.Compare(b.Price)
			if order == SortPriceDesc {
				cmp = -cmp
			}
			return cmp
		})
		start := min(page*pageSize, len(records))
		end := min(start+pageSize, len(records))
		hits := make([]map[string]any, 0, end-start)
		for _, box := range records[start:end] {
			hits = append(hits, map[string]any{"boxName": box.Name, "boxId": box.ID, "sellPrice": box.Price})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"hits": hits, "nbPages": (len(records) + 49) / 50}}})
	}))
	oldURL := URL
	URL = server.URL
	t.Cleanup(func() { URL = oldURL; server.Close() })
	return b
}

func testBoxes(prefix string, count, first, step int) []Box {
	records := make([]Box, count)
	for i := range records {
		name := fmt.Sprintf("%s item %04d", prefix, i)
		records[i] = Box{ID: fmt.Sprintf("%s-%04d", prefix, i), Name: name, Model: extractModelType(name), Price: decimal.NewFromInt(int64(first + i*step))}
	}
	return records
}

func TestPricePagesIncrementalAndCached(t *testing.T) {
	b := mockPriceBackend(t, map[string][]Box{"odd": testBoxes("odd", 500, 1, 2), "even": testBoxes("even", 500, 2, 2)})
	finder, _ := NewCexFind()
	for page := range 10 {
		boxes, more, err := finder.SearchPageSorted([]string{"odd", "even", "odd"}, false, "", page, SortPrice)
		if err != nil || len(boxes) != 50 || !more {
			t.Fatalf("page %d: len=%d more=%t err=%v", page, len(boxes), more, err)
		}
		for i, box := range boxes {
			if box.Price.IntPart() != int64(page*50+i+1) {
				t.Fatalf("wrong price page=%d item=%d: %s", page, i, box.Price)
			}
		}
	}
	b.mu.Lock()
	calls := len(b.calls["odd"]) + len(b.calls["even"])
	b.mu.Unlock()
	// 500 displayed hits plus at most one 50-hit lookahead batch per stream.
	if calls > 12 {
		t.Fatalf("too many upstream batches: %d", calls)
	}
	finder.SearchPageSorted([]string{"even", "odd"}, false, "", 0, SortPrice)
	finder.SearchPageSorted([]string{"odd", "even"}, false, "", 9, SortPrice)
	b.mu.Lock()
	defer b.mu.Unlock()
	if got := len(b.calls["odd"]) + len(b.calls["even"]); got != calls {
		t.Fatalf("cached pages made requests: %d -> %d", calls, got)
	}
	for query, pages := range b.calls {
		for i, page := range pages {
			if page != i {
				t.Fatalf("%s refetched/skipped a batch: %v", query, pages)
			}
		}
	}
}

func TestPricePagesDuplicatesStrictTiesAndDescending(t *testing.T) {
	for _, order := range []string{SortPrice, SortPriceDesc} {
		t.Run(order, func(t *testing.T) {
			a := testBoxes("alpha", 140, 1, 1)
			// All prices equal; each tie crosses several upstream pages.
			for i := range a {
				a[i].Price = decimal.NewFromInt(10)
				a[i].Name = fmt.Sprintf("alpha item %04d", 139-i)
				a[i].Model = extractModelType(a[i].Name)
			}
			beta := slices.Clone(a[:90])
			beta = append(beta, testBoxes("suggestion", 60, 1, 1)...)
			b := mockPriceBackend(t, map[string][]Box{"alpha": a, "beta": beta})
			finder, _ := NewCexFind()
			want := slices.Clone(a)
			SortBoxes(want, order)
			for page := range 3 {
				got, more, err := finder.SearchPageSorted([]string{"alpha", "beta"}, true, "", page, order)
				if err != nil {
					t.Fatal(err)
				}
				start := page * 50
				end := min(start+50, len(want))
				if len(got) != end-start || more != (end < len(want)) {
					t.Fatalf("page %d len=%d more=%t", page, len(got), more)
				}
				for i, box := range got {
					if box.ID != want[start+i].ID {
						t.Fatalf("unstable tie/dedup ordering: %s != %s", box.ID, want[start+i].ID)
					}
				}
			}
			if len(b.calls["alpha"]) != 3 || len(b.calls["beta"]) != 3 {
				t.Fatalf("unexpected batches: %v", b.calls)
			}
		})
	}
}

func TestPricePagesSkewAndDirectJump(t *testing.T) {
	b := mockPriceBackend(t, map[string][]Box{"cheap": testBoxes("cheap", 250, 1, 1), "expensive": testBoxes("expensive", 250, 10000, 1)})
	finder, _ := NewCexFind()
	got, more, err := finder.SearchPageSorted([]string{"cheap", "expensive"}, false, "", 3, SortPrice)
	if err != nil || len(got) != 50 || !more || got[0].Price.IntPart() != 151 || got[49].Price.IntPart() != 200 {
		t.Fatalf("direct jump got=%v more=%t err=%v", got, more, err)
	}
	if len(b.calls["expensive"]) != 1 {
		t.Fatalf("unneeded expensive pages: %v", b.calls)
	}
}

func TestPriceCacheExpiryFiltersAndFailure(t *testing.T) {
	b := mockPriceBackend(t, map[string][]Box{"alpha": testBoxes("alpha", 80, 1, 1), "beta": testBoxes("beta", 80, 1000, 1)})
	finder, _ := NewCexFind()
	b.failQuery = "beta"
	if _, _, err := finder.SearchPageSorted([]string{"alpha", "beta"}, false, "", 0, SortPrice); err == nil {
		t.Fatal("served incomplete ordering after failure")
	}
	b.mu.Lock()
	b.failQuery = ""
	b.mu.Unlock()
	if _, _, err := finder.SearchPageSorted([]string{"alpha", "beta"}, false, "", 0, SortPrice); err != nil {
		t.Fatal(err)
	}
	if len(b.calls["alpha"]) != 2 {
		t.Fatalf("successful batch was refetched: %v", b.calls)
	} // second alpha batch closes cutoff tie
	key := priceSearchKey([]string{"alpha", "beta"}, false, PriceRange{}, SortPrice)
	finder.priceSearches[key].expires = time.Now().Add(-time.Second)
	finder.SearchPageSorted([]string{"alpha", "beta"}, false, "", 0, SortPrice)
	if len(b.calls["beta"]) != 3 {
		t.Fatalf("expired cache was reused: %v", b.calls)
	}
	price, _ := ParsePriceRange("0", "100")
	finder.SearchPageSorted([]string{"alpha", "beta"}, false, "", 0, SortPrice, price)
	if len(finder.priceSearches) != 2 {
		t.Fatal("price filters shared a cache key")
	}
}

func TestPricePagesConcurrentCoalescing(t *testing.T) {
	b := mockPriceBackend(t, map[string][]Box{"alpha": testBoxes("alpha", 100, 1, 2), "beta": testBoxes("beta", 100, 2, 2)})
	finder, _ := NewCexFind()
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			got, _, err := finder.SearchPageSorted([]string{"alpha", "beta"}, false, "", 0, SortPrice)
			if err != nil || len(got) != 50 {
				t.Errorf("len=%d err=%v", len(got), err)
			}
		})
	}
	wg.Wait()
	if len(b.calls["alpha"])+len(b.calls["beta"]) != 2 {
		t.Fatalf("concurrent requests not coalesced: %v", b.calls)
	}
}

func TestCleanQueriesLimits(t *testing.T) {
	for _, queries := range [][]string{nil, {" "}, {"%zz"}, {"https://example.com"}, {"a", "b", "c", "d", "e", "f", "g", "h", "i"}} {
		if _, err := cleanQueries(queries); err == nil {
			t.Errorf("accepted invalid queries %v", queries)
		}
	}
}

func TestPricePagesBoundedCacheAndHitLimit(t *testing.T) {
	records := map[string][]Box{"alpha": testBoxes("alpha", 1200, 1, 1), "beta": testBoxes("beta", 1200, 10000, 1)}
	b := mockPriceBackend(t, records)
	finder, _ := NewCexFind()
	got, more, err := finder.SearchPageSorted([]string{"alpha", "beta"}, false, "", 999, SortPrice)
	if err != nil || len(got) != 0 || more {
		t.Fatalf("beyond limit: len=%d more=%t err=%v", len(got), more, err)
	}
	if len(b.calls["alpha"]) != 20 || len(b.calls["beta"]) != 20 {
		t.Fatalf("unbounded deep paging: %v", b.calls)
	}
	for i := range maxPriceSearches + 2 {
		records[fmt.Sprintf("query-%d", i)] = testBoxes("test", 1, 1, 1)
	}
	// These query names yield no hits; each cached search is still bounded.
	for i := range maxPriceSearches + 2 {
		finder.SearchPageSorted([]string{fmt.Sprintf("query-%d", i), "beta"}, false, "", 0, SortPrice)
		if len(finder.priceSearches) > maxPriceSearches {
			t.Fatalf("unbounded cache: %d", len(finder.priceSearches))
		}
	}
}

func TestPricePagesStrictFilteringLoadsPastEmptyBatch(t *testing.T) {
	alpha := testBoxes("suggestion", 60, 1, 1)
	alpha = append(alpha, testBoxes("alpha", 60, 61, 1)...)
	mockPriceBackend(t, map[string][]Box{"alpha": alpha, "beta": testBoxes("beta", 0, 1, 1)})
	finder, _ := NewCexFind()
	got, more, err := finder.SearchPageSorted([]string{"alpha", "beta"}, true, "", 0, SortPrice)
	if err != nil || len(got) != 50 || !more || got[0].Price.IntPart() != 61 {
		t.Fatalf("filtered batch: len=%d more=%t err=%v", len(got), more, err)
	}
}

func TestPricePageContextCancellation(t *testing.T) {
	b := mockPriceBackend(t, map[string][]Box{"alpha": testBoxes("alpha", 100, 1, 2), "beta": testBoxes("beta", 100, 2, 2)})
	finder, _ := NewCexFind()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := finder.SearchPageSortedContext(ctx, []string{"alpha", "beta"}, false, "", 0, SortPrice); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search: %v", err)
	}
	if len(b.calls) != 0 {
		t.Fatalf("cancelled search contacted backend: %v", b.calls)
	}
}
