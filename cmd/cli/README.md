# cli

A cli client to `cexfind`.

<img width="1000" src="./cli.gif" />

The animated gif was made with Charm's
[vhs](https://github.com/charmbracelet/vhs).

## Usage

```
Usage of ./cli:
  -page int
      result page to fetch (1-1000; 50 hits per query per page) (default 1)
  -postcode string
    	specify postcode
  -proxy string
    	proxy, eg: socks5://127.0.0.1:8080
  -query value
    	list of queries
  -sort string
      sort by model, price, price-desc, or distance (requires postcode) (default "model")
  -strict
    	only return items that strictly match the search terms
  -verbose
    	show verbose output, including cash/exchange prices and stores

a cli programme to search Cex/Webuy for second hand equipment

eg <programme> [-strict] [-page 2] -query "query 1" [-query "query 2"...]

```

Add `-sort price` for cheapest first, `-sort price-desc` for most expensive
first, or `-postcode "SW1A 0AA" -sort distance` for nearest store first.

Use `-page 2` to fetch the next page. Each run fetches only the selected
page, with up to 50 hits per query. Sorting applies to that page's results.
