package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"

	"github.com/rorycl/cexfind"
	"github.com/rorycl/cexfind/cmd"
)

// listenAndServe is an indirect of http/net.Server.ListenAndServe
var listenAndServe = (*http.Server).ListenAndServe

// production is default; set inDevelopment to true with build tag
var inDevelopment bool = false

// the server struct holds development flags and static and template
// directory locations
type server struct {
	WebMaxHeaderBytes int
	ServerAddress     string
	ServerPort        string
	ProxyURL          string
	BaseURL           string

	// searcher provides the search func to use, normally cex.Search, and whether
	// location/distance calculations are in use cex.LocationDistanceOK.
	searcher Searcher

	staticDirDev string
	tplDirDev    string
	staticDir    string
	tplDir       string
	DirFS        *fileSystem

	// serveFunc is an indirect for the main server functionality,
	// provided for testing
	serveFunc func()
}

type Searcher interface {
	Search(queries []string, strict bool, postcode string) ([]cexfind.Box, error)
	SearchPage(queries []string, strict bool, postcode string, page int, prices ...cexfind.PriceRange) ([]cexfind.Box, bool, error)
	SearchPageSorted(queries []string, strict bool, postcode string, page int, order string, prices ...cexfind.PriceRange) ([]cexfind.Box, bool, error)
	LocationDistancesOK() bool
}

// newServer configuress a new http server.
func newServer(addr, port, proxy string, searcher Searcher) (*server, error) {
	if addr == "" {
		addr = "127.0.0.1"
	}
	if port == "" {
		port = "8000"
	}

	s := server{
		// paths
		staticDirDev: "static",
		tplDirDev:    "templates",
		staticDir:    "static",
		tplDir:       "templates",

		// searcher is the passed-in search function.
		searcher: searcher,

		// WebMaxHeaderBytes is the largest number of header bytes accepted by
		// the webserver
		WebMaxHeaderBytes: 1 << 17, // ~125k

		// ServerAddress is the default Server network address
		ServerAddress: addr,

		// ServerPort is the default Server network port
		ServerPort: port,

		// BaseURL is the base url for redirects, etc.
		BaseURL: "",
	}
	// serveFunc is an indirect for testing
	s.serveFunc = s.serve
	return &s, nil
}

// setupFS setup the filesystem for templates or static files, depending on
// development (filesystem) or not (embedded)
func (s *server) setupFS() error {
	var err error
	if inDevelopment {
		s.DirFS, err = NewFileSystem(inDevelopment, s.tplDirDev, s.staticDirDev)
	} else {
		s.DirFS, err = NewFileSystem(inDevelopment, s.tplDir, s.staticDir)
	}
	return err
}

// Serve runs the web server on the specified address and port
func (s *server) Serve() {
	// setup the filesystem
	if err := s.setupFS(); err != nil {
		log.Fatal(err)
	}
	s.serveFunc()
}

func (s *server) serve() {
	// endpoint routing; gorilla mux is used because "/" in http.NewServeMux
	// is a catch-all pattern
	r := mux.NewRouter()

	// attach static dynamic file system to the http.FileServer
	// https://pkg.go.dev/github.com/gorilla/mux#section-readme :Static Files
	r.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/",
			http.FileServer(http.FS(s.DirFS.StaticFS))),
	)

	// routes
	r.HandleFunc("/results", s.Results)
	r.HandleFunc("/health", s.Health)
	r.HandleFunc("/favicon.ico", s.Favicon)
	r.HandleFunc("/", s.Home)

	// logging converts gorilla's handlers.CombinedLoggingHandler to a
	// func(http.Handler) http.Handler to satisfy type MiddlewareFunc
	logging := func(handler http.Handler) http.Handler {
		return handlers.CombinedLoggingHandler(os.Stdout, handler)
	}

	// recovery converts gorilla's handlers.RecoveryHandler to a
	// func(http.Handler) http.Handler to satisfy type MiddlewareFunc
	recovery := func(handler http.Handler) http.Handler {
		return handlers.RecoveryHandler()(handler)
	}

	// compression handler
	compressor := func(handler http.Handler) http.Handler {
		return handlers.CompressHandler(handler)
	}

	// attach middleware
	// r.Use(bodyLimitMiddleware)
	r.Use(logging)
	r.Use(compressor)
	r.Use(recovery)

	// configure server options
	server := &http.Server{
		Addr:    s.ServerAddress + ":" + s.ServerPort,
		Handler: r,
		// timeouts and limits
		MaxHeaderBytes:    s.WebMaxHeaderBytes,
		ReadTimeout:       1 * time.Second,
		WriteTimeout:      2 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}
	log.Printf("serving on %s:%s", s.ServerAddress, s.ServerPort)

	err := listenAndServe(server)
	if err != nil {
		log.Printf("fatal server error: %v", err)
	}
}

// Results shows the results of a "search" form submission in an htmx partial
func (s *server) Results(w http.ResponseWriter, r *http.Request) {

	if r.Method != "POST" {
		w.WriteHeader(http.StatusBadRequest)
		log.Print("endpoint only accepts POST requests, got", r.Method)
		return
	}

	// read body
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Print("results endpoint body reading error", err)
		return
	}
	if inDevelopment {
		log.Println("body content:", string(body))
	}

	// extract query from POSTed htmx form
	urlVals, err := url.ParseQuery(string(body))
	if err != nil {
		log.Printf("url parsequery error: %v", err)
		return
	}
	var postResults QueriesType
	var decoder = schema.NewDecoder() // best as package decoder
	err = decoder.Decode(&postResults, urlVals)
	if err != nil {
		http.Error(w, "invalid search parameters", http.StatusBadRequest)
		return
	}
	if len(postResults.Query) == 0 {
		log.Printf("cex POST : %+v %v", postResults, err)
		w.WriteHeader(http.StatusNoContent)
		fmt.Fprint(w, "no query found")
		return
	}

	// split the comma delimited query into queries
	queries, err := cmd.QueryInputChecker(postResults.Query...)
	if err != nil {
		log.Printf("cex queries error: %v %v", postResults.Query, err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "query error: %v", err)
		return
	}
	if postResults.Page < 0 || postResults.Page > 999 {
		http.Error(w, "page must be between 0 and 999", http.StatusBadRequest)
		return
	}
	price, err := cexfind.ParsePriceRange(postResults.MinPrice, postResults.MaxPrice)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	base := fmt.Sprintf("strict=%s", func() string {
		if postResults.Strict {
			return "true"
		}
		return "false"
	}())
	if postResults.Postcode != "" {
		base += fmt.Sprintf("&postcode=%s", url.PathEscape(postResults.Postcode))
	}
	if price.Min != nil {
		base += "&min_price=" + url.QueryEscape(price.Min.String())
	}
	if price.Max != nil {
		base += "&max_price=" + url.QueryEscape(price.Max.String())
	}
	postResults.Sort = validSort(postResults.Sort, postResults.Postcode, s.searcher.LocationDistancesOK())
	if postResults.Sort != "model" {
		base += fmt.Sprintf("&sort=%s", postResults.Sort)
	}
	if postResults.Page > 0 {
		base += fmt.Sprintf("&page=%d", postResults.Page)
	}
	for _, q := range queries {
		base += fmt.Sprintf("&query=%s", url.PathEscape(q))
	}
	// push the postResults terms to the url
	w.Header().Set("HX-Push-Url", s.BaseURL+"/?"+base)

	// search; note that searcher is an indirect to search/cex.Search
	type SearchResults struct {
		Results      []cexfind.Box
		Err          error
		Sort         string
		Page         int
		PreviousPage int
		NextPage     int
		HasPrevious  bool
		HasNext      bool
	}
	sr := SearchResults{Sort: postResults.Sort, Page: postResults.Page + 1, PreviousPage: postResults.Page - 1, NextPage: postResults.Page + 1, HasPrevious: postResults.Page > 0}
	sr.Results, sr.HasNext, sr.Err = s.searcher.SearchPageSorted(queries, postResults.Strict, postResults.Postcode, postResults.Page, sr.Sort, price)
	cexfind.SortBoxes(sr.Results, sr.Sort)

	t := template.Must(template.ParseFS(s.DirFS.TplFS, "partial-results.html"))
	err = t.Execute(w, sr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "template writing problem : %s", err.Error())
	}
}

type QueriesType struct {
	Postcode string   `schema:"postcode"`
	MinPrice string   `schema:"min_price"`
	MaxPrice string   `schema:"max_price"`
	Strict   bool     `schema:"strict"`
	Sort     string   `schema:"sort"`
	Page     int      `schema:"page"`
	Query    []string `schema:"query"`
}

func validSort(sort, postcode string, distancesOK bool) string {
	switch sort {
	case "price", "price-desc":
		return sort
	case "distance":
		if postcode != "" && distancesOK {
			return sort
		}
	}
	return "model"
}

// nearestDistance returns the closest store with known coordinates.
// String provides a string representation of QueriesType.Query,
// suitable for use in a template
func (q QueriesType) String() string {
	output := ""
	for i, query := range q.Query {
		if i > 0 {
			output += cmd.QuerySplitChar + " "
		}
		output += query
	}
	return output
}

// Home is the home page
func (s *server) Home(w http.ResponseWriter, r *http.Request) {

	t, err := template.ParseFS(s.DirFS.TplFS, "home.html")
	if err != nil {
		log.Fatal(err)
	}

	var search QueriesType
	var decoder = schema.NewDecoder() // best as package decoder
	err = decoder.Decode(&search, r.URL.Query())
	if price, priceErr := cexfind.ParsePriceRange(search.MinPrice, search.MaxPrice); priceErr == nil {
		if price.Min != nil {
			search.MinPrice = price.Min.String()
		}
		if price.Max != nil {
			search.MaxPrice = price.Max.String()
		}
	} else {
		search.MinPrice, search.MaxPrice = "", ""
	}
	search.Sort = validSort(search.Sort, search.Postcode, s.searcher.LocationDistancesOK())

	if inDevelopment {
		log.Printf("cex url GET : %+v %+v (%d items) err %v", r.URL.Query(), search, len(search.Query), err)
	}

	data := struct {
		Title               string
		Address             string
		Port                string
		Search              QueriesType
		LocationDistancesOK bool
	}{
		"search cex",
		s.ServerAddress,
		s.ServerPort,
		search,
		s.searcher.LocationDistancesOK(),
	}
	err = t.Execute(w, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "template writing problem : %s", err.Error())
	}
}

// HealthCheck shows if the service is up
func (s *server) Health(w http.ResponseWriter, r *http.Request) {
	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := map[string]string{"status": "up"}
	if err := enc.Encode(resp); err != nil {
		log.Print("health error: unable to encode response")
	}
}

// Favicon serves up the favicon
func (s *server) Favicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, s.DirFS.StaticFS, "/favicon.svg")
}
