package cexfind

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rorycl/cexfind/location"
	"github.com/shopspring/decimal"
)

func TestSearchPageRequestsOneUpstreamPage(t *testing.T) {
	oldURL := URL
	defer func() { URL = oldURL }()

	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body struct {
			Requests []struct {
				Params string `json:"params"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if len(body.Requests) != 1 {
			t.Errorf("got %d upstream requests, want 1", len(body.Requests))
			return
		}
		params, err := url.ParseQuery(body.Requests[0].Params)
		if err != nil {
			t.Errorf("parse params: %v", err)
			return
		}
		if got := params.Get("hitsPerPage"); got != "50" {
			t.Errorf("hitsPerPage = %q, want 50", got)
		}
		if got := params.Get("page"); got != "1" {
			t.Errorf("page = %q, want 1", got)
		}
		fmt.Fprint(w, `{"results":[{"hits":[{"boxName":"Test item","boxId":"item-2","sellPrice":10}],"nbPages":3}]}`)
	}))
	defer ts.Close()
	URL = ts.URL

	cex, err := NewCexFind()
	if err != nil {
		t.Fatal(err)
	}
	boxes, hasMore, err := cex.SearchPage([]string{"test item"}, false, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(boxes) != 1 || boxes[0].ID != "item-2" || !hasMore {
		t.Errorf("requests=%d boxes=%v hasMore=%t", requests, boxes, hasMore)
	}
}

// TestBoxInQuery tests strict query/Box.Name matches
func TestBoxInQuery(t *testing.T) {

	tests := []struct {
		box     Box
		queries []string
		result  bool
	}{
		{
			box:     Box{Name: "ABC def hij"},
			queries: []string{"xyz ntz", "hij abc"},
			result:  true,
		},
		{
			box:     Box{Name: "ABC def hij"},
			queries: []string{"xyz ntz", "hij dbc"},
			result:  false,
		},
		{
			box:     Box{Name: "abc def hij"},
			queries: []string{"HIJ ABC"},
			result:  true,
		},
		{
			box:     Box{Name: "abc def hij", Model: "lenovo"},
			queries: []string{"HIJ ABC Lenovo"},
			result:  true,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("subtest %d", i), func(t *testing.T) {
			if got, want := tt.box.inQuery(tt.queries), tt.result; got != want {
				t.Errorf("got %t != want %t", got, want)
			}
		})
	}
}

func TestSearch(t *testing.T) {

	f, err := os.Open("testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, string(contents))
	}))
	defer ts.Close()

	// overwrite global URL with test URL
	URL = ts.URL

	// non-strict search
	cex, err := NewCexFind()
	if err != nil {
		t.Fatalf("unexpected init error: %s", err)
	}
	results, err := cex.Search([]string{"lenovo x390s"}, false, "")
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(results), 6; got != want {
		t.Fatalf("expected %d box results, got %d", want, got)
	}

	// verbose output (use test -v)
	for _, v := range results {
		t.Log("\t", v)
	}

	// strict search for non-existing model
	cex, err = NewCexFind()
	if err != nil {
		t.Fatalf("unexpected init error: %s", err)
	}
	_, err = cex.Search([]string{"lenovo x390st"}, true, "")
	if err == nil || err.Error() != "no results" {
		t.Fatalf("expected no results error, got %v", err)
	}
}

// Search for terminator search string
func TestSearchTerminator(t *testing.T) {

	f, err := os.Open("testdata/terminator.json")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, string(contents))
	}))
	defer ts.Close()

	// overwrite global URL with test URL
	URL = ts.URL

	// non-strict search
	cex, err := NewCexFind()
	if err != nil {
		t.Fatalf("unexpected init error: %s", err)
	}
	results, err := cex.Search([]string{"terminator"}, false, "")
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(results), 17; got != want {
		t.Fatalf("expected %d box results, got %d", want, got)
	}

	// verbose output (use test -v)
	for _, v := range results {
		t.Log("\t", v)
	}
}

// TestBoxSort tests box sorting
func TestBoxSort(t *testing.T) {

	var toSortBoxes boxes
	toSortBoxes = append(toSortBoxes,

		[]Box{
			{Model: "bb", Name: "bb", ID: "id1a", Price: decimal.NewFromInt(20), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)},
			{Model: "bc", Name: "cc", ID: "id2a", Price: decimal.NewFromInt(25), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)},
			{Model: "ba", Name: "aa", ID: "id3a", Price: decimal.NewFromInt(15), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)},
			{Model: "ab", Name: "db", ID: "id3b", Price: decimal.NewFromInt(30), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)},
			{Model: "ac", Name: "dc", ID: "id2z", Price: decimal.NewFromInt(35), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)},
			{Model: "aa", Name: "da", ID: "id1a", Price: decimal.NewFromInt(35), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)},
			{Model: "aa", Name: "la", ID: "id1b", Price: decimal.NewFromInt(30), PriceCash: decimal.NewFromInt(15), PriceExchange: decimal.NewFromInt(17)}, // 0
		}...,
	)

	var sortedBoxes = make(boxes, len(toSortBoxes))
	copy(sortedBoxes, toSortBoxes)

	sortedBoxes.sort()

	// t.Logf("\n%d: %v\n", len(toSortBoxes), toSortBoxes)
	// t.Logf("\n%d: %v\n", len(sortedBoxes), sortedBoxes)
	// t.Log(sortedBoxes)

	// compaction does not happen here
	if got, want := len(sortedBoxes), len(toSortBoxes); got != want {
		t.Errorf("expected compaction want %d items, got %d", want, got)
	}

	if diff := cmp.Diff(
		toSortBoxes[6],
		sortedBoxes[0],
		cmpopts.IgnoreFields(Box{}, "storeNames"),
	); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(
		toSortBoxes[0],
		sortedBoxes[5],
		cmpopts.IgnoreFields(Box{}, "storeNames"),
	); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestBoxStoresString(t *testing.T) {
	tests := []struct {
		length    int
		boxString []string
		want      string
	}{
		{
			length:    6,
			boxString: []string{},
			want:      "",
		},
		{
			length:    7,
			boxString: []string{"a", "b", "c"},
			want:      "a, b, c",
		},
		{
			length:    5,
			boxString: []string{"a", "b", "c"},
			want:      "a, b…",
		},
		{
			length:    14,
			boxString: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
			want:      "a, b, c, d, e…",
		},
	}
	for i, tt := range tests {
		box := Box{ID: "whatever"}
		for _, bs := range tt.boxString {
			box.Stores = append(box.Stores, location.StoreWithDistance{
				StoreName: bs,
			})
		}
		t.Run(fmt.Sprintf("subtest %d", i), func(t *testing.T) {
			if got, want := box.StoresString(tt.length), tt.want; got != want {
				t.Errorf("got %s != want %s", got, want)
			}
		})
	}
}

// TestBoxIDUrl checks a valid url is returned
func TestBoxIDUrl(t *testing.T) {
	b := Box{ID: "xyz"}
	if got, want := b.IDUrl(), urlDetail+b.ID; got != want {
		t.Errorf("url got %s want %s", got, want)
	}
}

func TestCexInitialised(t *testing.T) {
	nsd, err := location.NewStoreDistances(http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	cex := &CexFind{
		storeDistances: nsd,
	}
	if got, want := cex.LocationDistancesOK(), false; got != want {
		t.Errorf("got %t want %t", got, want)
	}
}

// Test proxy schema
func TestProxySchema(t *testing.T) {
	tests := []struct {
		URL string
		ok  bool
	}{
		{"socks5://127.0.0.1:8081", true},
		{"socks7://127.0.0.1:8081", false},
		{"https://127.0.0.2:8080", true},
		{"ftp://127.0.0.2:8080", false},
		{"", false},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			u, err := url.Parse(tt.URL)
			if err != nil {
				t.Fatal(err) // setup failure
			}
			err = proxySchemeOK(u)
			if err == nil && !tt.ok {
				t.Errorf("expected error for %q", tt.URL)
			}
			if err != nil && tt.ok {
				t.Errorf("unexpected error %s for %q", err, tt.URL)
			}
		})
	}
}
