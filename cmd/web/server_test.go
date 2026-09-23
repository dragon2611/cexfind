package main

// https://ieftimov.com/posts/testing-in-go-testing-http-servers/
// https://bignerdranch.com/blog/using-the-httptest-package-in-golang/

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rorycl/cexfind"
	"github.com/rorycl/cexfind/location"
	"github.com/shopspring/decimal"
)

type srch struct {
	f         func() ([]cexfind.Box, error)
	pageFunc  func(int) ([]cexfind.Box, bool, error)
	priceFunc func(cexfind.PriceRange)
	locDist   bool
}

func (s *srch) Search(queries []string, strict bool, postcode string) ([]cexfind.Box, error) {
	if s.f == nil {
		return []cexfind.Box{}, nil
	} else {
		return s.f()
	}
}
func (s *srch) SearchPage(queries []string, strict bool, postcode string, page int, prices ...cexfind.PriceRange) ([]cexfind.Box, bool, error) {
	if s.priceFunc != nil && len(prices) == 1 {
		s.priceFunc(prices[0])
	}
	if s.pageFunc != nil {
		return s.pageFunc(page)
	}
	results, err := s.Search(queries, strict, postcode)
	return results, false, err
}

func TestResultsPriceRange(t *testing.T) {
	var got cexfind.PriceRange
	s, err := newServer("", "", "", &srch{
		priceFunc: func(price cexfind.PriceRange) { got = price },
		pageFunc: func(page int) ([]cexfind.Box, bool, error) {
			return []cexfind.Box{{ID: "item", Model: "Test", Price: decimal.NewFromInt(50)}}, true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.DirFS = &fileSystem{TplFS: os.DirFS("templates")}
	w := httptest.NewRecorder()
	s.Results(w, httptest.NewRequest(http.MethodPost, "/results", strings.NewReader("query=test&min_price=25.50&max_price=100&page=1")))
	if w.Code != http.StatusOK || got.Min == nil || got.Max == nil || got.Min.String() != "25.5" || got.Max.String() != "100" {
		t.Errorf("status=%d, price=%+v, body=%s", w.Code, got, w.Body.String())
	}
	for _, want := range []string{"min_price=25.5", "max_price=100", "page=1"} {
		if !strings.Contains(w.Header().Get("HX-Push-Url"), want) {
			t.Errorf("push URL %q missing %q", w.Header().Get("HX-Push-Url"), want)
		}
	}

	w = httptest.NewRecorder()
	s.Home(w, httptest.NewRequest(http.MethodGet, "/?query=test&min_price=25.5&max_price=100", nil))
	for _, want := range []string{`name="min_price" min="0" step="0.01" value="25.5"`, `name="max_price" min="0" step="0.01" value="100"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("home missing %q", want)
		}
	}

	for _, body := range []string{"query=test&min_price=-1", "query=test&min_price=abc", "query=test&min_price=101&max_price=100"} {
		w = httptest.NewRecorder()
		s.Results(w, httptest.NewRequest(http.MethodPost, "/results", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q status %d, want 400", body, w.Code)
		}
	}
}

func TestResultsPagination(t *testing.T) {
	requestedPage := -1
	s, err := newServer("", "", "", &srch{pageFunc: func(page int) ([]cexfind.Box, bool, error) {
		requestedPage = page
		return []cexfind.Box{{ID: "second-page", Model: "Test", Name: "Test item"}}, true, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	s.DirFS = &fileSystem{TplFS: os.DirFS("templates")}
	w := httptest.NewRecorder()
	s.Results(w, httptest.NewRequest(http.MethodPost, "/results", strings.NewReader("query=test&page=1")))
	if requestedPage != 1 || w.Code != http.StatusOK {
		t.Fatalf("requested page %d, status %d", requestedPage, w.Code)
	}
	for _, want := range []string{`page=1`, `Page 2`, `"page":0`, `"page":2`, `second-page`} {
		content := w.Body.String()
		if want == "page=1" {
			content = w.Header().Get("HX-Push-Url")
		}
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in %q", want, content)
		}
	}

	for _, page := range []string{"-1", "1000", "invalid"} {
		w = httptest.NewRecorder()
		s.Results(w, httptest.NewRequest(http.MethodPost, "/results", strings.NewReader("query=test&page="+page)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("page %q status %d, want 400", page, w.Code)
		}
	}
}

func TestHomeLoadsSelectedPage(t *testing.T) {
	s, err := newServer("", "", "", &srch{})
	if err != nil {
		t.Fatal(err)
	}
	s.DirFS = &fileSystem{TplFS: os.DirFS("templates")}
	w := httptest.NewRecorder()
	s.Home(w, httptest.NewRequest(http.MethodGet, "/?query=test&page=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	for _, want := range []string{`hx-trigger="load"`, `"page":1`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %q in home page", want)
		}
	}
}
func (s *srch) LocationDistancesOK() bool {
	return s.locDist
}

// newTestSearcher returns an object fulfilling the Searcher interface.
func newTestSearcher() Searcher {
	return &srch{}
}

// TestSetupFS sets up the FS
func TestSetupFS(t *testing.T) {
	s, err := newServer("127.0.0.1", "8000", "socks5://127.0.0.1:8002", newTestSearcher())
	if err != nil {
		t.Fatal(err)
	}

	s.staticDirDev = "static"
	s.tplDirDev = "templates"
	if got, want := s.setupFS(), error(nil); got != want {
		t.Errorf("testsetupfs error got %v != want %v", got, want)
	}
}

// TestServe
func TestServe(t *testing.T) {
	s, err := newServer("", "", "", newTestSearcher())
	if err != nil {
		t.Fatal(err)
	}

	s.serveFunc = func() {}
	s.staticDirDev = "static"
	s.tplDirDev = "templates"
	s.Serve()
}

// Test Home page returns a 200
func TestHome(t *testing.T) {

	s, err := newServer("127.0.0.1", "8003", "socks5://127.0.0.1:8003", newTestSearcher())
	if err != nil {
		t.Fatal(err)
	}
	// home uses templates fs
	s.DirFS = &fileSystem{}
	s.DirFS.TplFS = os.DirFS("templates")

	r := httptest.NewRequest(http.MethodGet, "http://example.com/home", nil)
	w := httptest.NewRecorder()

	s.Home(w, r)

	res := w.Result()
	defer res.Body.Close()
	_, err = io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	if want, got := 200, res.StatusCode; want != got {
		t.Errorf("expected status %d, got %d", want, got)
	}
}

func TestHomeSortOptions(t *testing.T) {
	for _, tc := range []struct {
		name, url, option string
	}{
		{"no postcode", "http://example.com/", `<option value="distance"  disabled>`},
		{"postcode and distance sort", "http://example.com/?postcode=SW1A+0AA&sort=distance", `<option value="distance" selected >`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := newServer("", "", "", &srch{locDist: true})
			if err != nil {
				t.Fatal(err)
			}
			s.DirFS = &fileSystem{TplFS: os.DirFS("templates")}
			w := httptest.NewRecorder()
			s.Home(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if !strings.Contains(w.Body.String(), tc.option) {
				t.Errorf("distance option %q not found in home page", tc.option)
			}
		})
	}
}

// Test Health page returns a 200
func TestHealth(t *testing.T) {

	r := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	w := httptest.NewRecorder()

	s, err := newServer("127.0.0.1", "8000", "", newTestSearcher())
	if err != nil {
		t.Fatal(err)
	}

	s.Health(w, r)

	res := w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	if want, got := 200, res.StatusCode; want != got {
		t.Errorf("expected status %d, got %d", want, got)
	}
	responseBody := string(data)
	if want, got := strings.TrimSpace(`{"status":"up"}`), strings.TrimSpace(responseBody); want != got {
		t.Errorf("expected status %s, got %s", want, got)
	}
}

// Favicon page returns a 200
func TestFavicon(t *testing.T) {

	s, err := newServer("127.0.0.1", "8000", "", newTestSearcher())
	if err != nil {
		t.Fatal(err)
	}

	// favicon uses the static fs
	s.DirFS = &fileSystem{}
	s.DirFS.StaticFS = os.DirFS("static")

	r := httptest.NewRequest(http.MethodGet, "http://example.com/favicon.ico", nil)
	w := httptest.NewRecorder()

	s.Favicon(w, r)

	res := w.Result()
	defer res.Body.Close()
	_, err = io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	if want, got := 200, res.StatusCode; want != got {
		t.Errorf("expected status %d, got %d", want, got)
	}
}

// TestResults tests a POST to Results; note that cexfind.Search is
// swapped out
func TestResults(t *testing.T) {

	/*
		type srch struct {}
		func (s *srch)
	*/

	s, err := newServer("127.0.0.1", "8000", "", newTestSearcher())
	if err != nil {
		t.Fatal(err)
	}

	// results uses the templates endpoint
	s.DirFS = &fileSystem{}
	s.DirFS.TplFS = os.DirFS("templates")

	// override searcher.Search
	ss := &srch{}
	ss.f = func() ([]cexfind.Box, error) {
		return []cexfind.Box{
			cexfind.Box{Model: "2a", Name: "2a name", ID: "id3", Price: decimal.NewFromInt(3)},
			cexfind.Box{Model: "1a", Name: "1a name", ID: "id1", Price: decimal.NewFromInt(1)},
			cexfind.Box{Model: "1b", Name: "1b name", ID: "id2", Price: decimal.NewFromInt(2)},
		}, nil
	}
	ss.locDist = false
	s.searcher = ss

	tt := []struct {
		name       string
		method     string
		input      string
		statusCode int
	}{
		{
			name:       "succeed post",
			method:     http.MethodPost,
			input:      "query=abc&query=def&strict=false",
			statusCode: http.StatusOK,
		},
		{
			name:       "fail post query too short",
			method:     http.MethodPost,
			input:      "query=ab&strict=false",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "fail post query too short 2",
			method:     http.MethodPost,
			input:      "query=abc&query=de&strict=false",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "fail get",
			method:     http.MethodGet,
			input:      "query=abc&query=def&strict=false",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "fail no POST body",
			method:     http.MethodPost,
			input:      "",
			statusCode: http.StatusNoContent,
		},
	}

	for _, tc := range tt {
		t.Logf("%+v\n", tc)
		t.Run(tc.name, func(t *testing.T) {

			r := httptest.NewRequest(tc.method, "http://example.com/request", strings.NewReader(tc.input))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			s.Results(w, r)

			res := w.Result()
			defer res.Body.Close()
			_, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			if tc.statusCode != res.StatusCode {
				t.Errorf("expected status %d, got %d", tc.statusCode, res.StatusCode)
			}

		})
	}
}

func TestSortResults(t *testing.T) {
	boxes := []cexfind.Box{
		{ID: "expensive", Model: "a", Price: decimal.NewFromInt(100), Stores: []location.StoreWithDistance{{StoreID: 1, DistanceMiles: 20}}},
		{ID: "unknown", Model: "b", Price: decimal.NewFromInt(30), Stores: []location.StoreWithDistance{{StoreName: "Unknown", DistanceMiles: 0}}},
		{ID: "near", Model: "c", Price: decimal.NewFromInt(50), Stores: []location.StoreWithDistance{{StoreID: 2, DistanceMiles: 8}, {StoreID: 3, DistanceMiles: 2}}},
	}
	tests := []struct {
		order string
		want  []string
	}{
		{"price", []string{"unknown", "near", "expensive"}},
		{"price-desc", []string{"expensive", "near", "unknown"}},
		{"distance", []string{"near", "expensive", "unknown"}},
	}
	for _, tc := range tests {
		t.Run(tc.order, func(t *testing.T) {
			got := append([]cexfind.Box(nil), boxes...)
			cexfind.SortBoxes(got, tc.order)
			for i, box := range got {
				if box.ID != tc.want[i] {
					t.Errorf("position %d: got %s, want %s", i, box.ID, tc.want[i])
				}
			}
		})
	}
}

func TestResultsSortSelection(t *testing.T) {
	s, err := newServer("", "", "", &srch{
		locDist: true,
		f: func() ([]cexfind.Box, error) {
			return []cexfind.Box{
				{ID: "far", Model: "a", Price: decimal.NewFromInt(10), Stores: []location.StoreWithDistance{{StoreID: 1, DistanceMiles: 20}}},
				{ID: "near", Model: "b", Price: decimal.NewFromInt(20), Stores: []location.StoreWithDistance{{StoreID: 2, DistanceMiles: 2}}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.DirFS = &fileSystem{TplFS: os.DirFS("templates")}

	for _, tc := range []struct {
		name, body, pushedSort, firstID string
	}{
		{"distance with postcode", "query=abc&postcode=SW1A+0AA&sort=distance", "sort=distance", "near"},
		{"distance without postcode", "query=abc&sort=distance", "", "far"},
		{"price descending", "query=abc&sort=price-desc", "sort=price-desc", "near"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/results", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			s.Results(w, r)
			res := w.Result()
			defer res.Body.Close()
			content, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status %d: %s", res.StatusCode, content)
			}
			if !strings.Contains(res.Header.Get("HX-Push-Url"), tc.pushedSort) {
				t.Errorf("push URL %q does not contain %q", res.Header.Get("HX-Push-Url"), tc.pushedSort)
			}
			if tc.pushedSort == "" && strings.Contains(res.Header.Get("HX-Push-Url"), "sort=") {
				t.Errorf("unexpected sort in push URL: %q", res.Header.Get("HX-Push-Url"))
			}
			first := strings.Index(string(content), ">"+tc.firstID+"</a>")
			if first < 0 {
				t.Fatalf("first ID %q missing from response", tc.firstID)
			}
			otherID := "far"
			if tc.firstID == "far" {
				otherID = "near"
			}
			if other := strings.Index(string(content), ">"+otherID+"</a>"); other < 0 || first > other {
				t.Errorf("wrong result order: %s", content)
			}
		})
	}
}
