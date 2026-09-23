// A cli client to github.com/rorycl/cexfind
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/rorycl/cexfind"
	"github.com/rorycl/cexfind/cmd"
)

var usage = `
a cli programme to search Cex/Webuy for second hand equipment

eg <programme> [-strict] [-page 2] -query "query 1" [-query "query 2"...]

`

// styles
var (
	urlStyle  = color.New(color.FgCyan).SprintFunc()
	dotStyle  = color.New(color.FgCyan).SprintFunc()
	infoStyle = color.New(color.FgMagenta).SprintFunc()
)

// queriesType is a flag list type
type queriesType []string

// set appends a string to a queriesType
func (q *queriesType) Set(s string) error {
	*q = append(*q, s)
	return nil
}

// String is needed for flag.Var
func (q *queriesType) String() string {
	return fmt.Sprintln(*q)
}

// indirect Exit for testing
var Exit func(code int) = os.Exit

// flagGetter indirects flagGet for testing
var flagGetter func() (queriesType, bool, string, string, bool, int, string) = flagGet

// flagGet checks the flags
func flagGet() (queriesType, bool, string, string, bool, int, string) {

	var (
		strict   bool
		queries  queriesType
		postCode string
		verbose  bool
		proxy    string
		page     int
		sortBy   string
	)

	flag.BoolVar(&strict, "strict", false, "only return items that strictly match the search terms")
	flag.Var(&queries, "query", "list of queries")
	flag.BoolVar(&verbose, "verbose", false, "show verbose output, including cash/exchange prices and stores")
	flag.StringVar(&postCode, "postcode", "", "specify postcode")
	flag.StringVar(&proxy, "proxy", "", "proxy, eg: socks5://127.0.0.1:8080")
	flag.IntVar(&page, "page", 1, "result page to fetch (1-1000; 50 hits per query per page)")
	flag.StringVar(&sortBy, "sort", cexfind.SortModel, "sort by model, price, price-desc, or distance (requires postcode)")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), usage)
	}

	flag.Parse()
	if len(queries) < 1 {
		flag.Usage()
		Exit(1)
	}
	postCode = strings.TrimSpace(postCode)
	switch sortBy {
	case cexfind.SortModel, cexfind.SortPrice, cexfind.SortPriceDesc:
	case cexfind.SortDistance:
		if postCode == "" {
			fmt.Fprintln(flag.CommandLine.Output(), "distance sorting requires -postcode")
			Exit(1)
		}
	default:
		fmt.Fprintf(flag.CommandLine.Output(), "invalid sort order %q\n", sortBy)
		Exit(1)
	}

	return queries, strict, postCode, proxy, verbose, page, sortBy
}

func main() {

	queries, strict, postCode, proxy, verbose, page, sortBy := flagGetter()
	if page < 1 || page > 1000 {
		fmt.Println("page must be between 1 and 1000")
		Exit(1)
		return
	}

	// clean queries
	queries, err := cmd.QueryInputChecker(queries...)
	if err != nil {
		fmt.Println(err)
		Exit(1)
	}

	// do search
	var options []cexfind.Option
	if proxy != "" {
		options = append(options, cexfind.WithProxy(proxy))
	}
	if postCode != "" {
		options = append(options, cexfind.WithStoreDistanceInitiliase())
	}
	cex, err := cexfind.NewCexFind(options...)
	if err != nil {
		fmt.Println(err)
		Exit(1)
	}

	results, _, err := cex.SearchPage(queries, strict, postCode, page-1)
	switch {
	case err != nil && len(results) > 0:
		fmt.Println(err)
		// continue to show the list
	case err != nil:
		fmt.Println(err)
		Exit(1)
	default:
		// show the list
	}
	cexfind.SortBoxes(results, sortBy)

	if verbose || postCode != "" {
		// print header
		fmt.Print("showing (cash/exchange price) and stores list")
		if postCode != "" {
			if !cex.LocationDistancesOK() {
				fmt.Print("\nnote: distance calculations failed.")
			} else {
				fmt.Print(", distance to stores in miles.")
			}
		}
		fmt.Println("")
	}

	k := ""
	for _, box := range results {
		if sortBy != cexfind.SortModel || box.Model != k {
			fmt.Printf("\n%s\n", box.Model)
			k = box.Model
		}
		if verbose || postCode != "" {
			info := fmt.Sprintf("(%d/%d) %s",
				box.PriceCash.IntPart(),
				box.PriceExchange.IntPart(),
				box.StoresString(80),
			)
			fmt.Printf(
				"%s %-3d %s [%s]\n      %s\n      %s\n",
				dotStyle("✱"),
				box.Price.IntPart(),
				box.Name,
				box.Category,
				// box.ID,
				urlStyle(box.IDUrl()),
				infoStyle(info),
			)
		} else {
			fmt.Printf(
				"%s %-3d %s [%s]\n      %s\n",
				dotStyle("✱"),
				box.Price.IntPart(),
				box.Name,
				box.Category,
				// box.ID,
				urlStyle(box.IDUrl()),
			)

		}
	}
}
