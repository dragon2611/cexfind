# cexfind

v0.3.2 : 24 August 2026 : add proxying

HTTP Proxy support is now provided in the library and cli, console and
web clients. This is convenient to use when out of the UK, for example
with a `ssh -D 8081 user@ukserver.com` type of socks5 proxy.

## Find kit on Cex, fast

<img width="1000" src="cmd/web/static/web-detail.png" />

This project is a Go module with console, cli and web app clients for
rapid and effective searches for second hand equipment for sale at
Cex/Webuy using the unofficial `webuy.io` json search endpoint.

Note that these programs only work for queries made in the UK (or via a
proxy terminating in the UK). This is intended to be a fun project and
is not intended for commercial use.

## Search features

- Search for several terms at once and use strict matching to exclude
  suggestions that do not contain the search terms.
- Fetch more than the first page of results in the CLI, web app, or Go module.
  Each query returns up to 50 hits per page; strict matching and duplicate
  removal can reduce the number shown.
- Sort the results on the selected page by model (the default), price from
  low to high, price from high to low, or distance to the nearest store.
  Distance sorting requires a postcode and available store location data.
- Limit results to an inclusive minimum and/or maximum selling price in pounds.

## Backend request limits

Combined price searches fetch fixed 50-hit batches and merge them incrementally.
Later pages reuse earlier batches instead of downloading growing prefixes of every
query. Only queries whose unseen prices could affect the requested page are
expanded. Equal-price results are resolved across batch boundaries before paging.

A finder retains up to eight combined price searches for two minutes from the
start of each search. Results may reflect that snapshot until it expires. Each
search accepts up to eight distinct query terms; repeated identical terms are
sent only once. Combined price searches examine at most 1,000 upstream hits per
query, including hits discarded by strict matching. Refine the query or price
range to search beyond that limit.

Search requests across all finders in a process start at least 500 ms apart, with
only one request waiting for response headers at a time. HTTP 429 responses stop
queued searches and return a cooldown error, honoring `Retry-After` (seconds or an
HTTP date), or using one minute if it is absent or invalid. There are no automatic
retries. Separate CLI invocations have separate caches and limiters; use a
long-running web or console client to reuse results between pages.

The web client cancels work when the browser disconnects and limits a search to
90 seconds. Library callers can use `SearchPageSortedContext` for cancellation.

## Usage

Simply download the binaries for your machine's architecture from
[releases](https://github.com/rorycl/cexfind/releases). Alternatively,
build for your local machine using `make build-all` if you have go (>=
1.27) installed. The resulting binaries can be found in `bin`.
The features described here reflect the current source tree; release binaries
may be older.

To build the Linux clients with Docker, run `./bin/docker-build.sh`.
It writes `webserver`, `cli`, and `console` to `deploy/` for the Docker builder's architecture; the
`deploy/` directory is ignored by Git. Set `DOCKER_PLATFORM=linux/amd64`
when building for an x86-64 Linux server.

For macOS, run `./bin/docker-build-mac.sh`. It writes `webserver`, `cli`,
and `console` to `deploy/macos-arm64/` on Apple Silicon or
`deploy/macos-amd64/` on Intel Macs. Pass `amd64` or `arm64` to the script
to select a different Mac architecture.

## Clients

Three clients are provided:

**web server**

A simple htmx webserver client. Enter multiple search terms separated by
semicolons, then choose a sort order. Use Previous and Next to move between
result pages. Search settings and the current page are kept in the URL so a
search can be revisited or shared.

Run `./bin/webserver` or the windows alternative to run the server
locally on the default local ip address of `127.0.0.1` and port `8000`.
Use the command line switches to change these options. (Use `-h` to see
the switches.) See the [web server README](cmd/web/README.md) for details.

<img width="1000" src="cmd/web/web.gif" />

**console**

A [bubbletea](https://github.com/charmbracelet/bubbletea) console app.

<img width="1000" src="cmd/console/console.gif" />

Have a look at the app [README](cmd/console/README.md) for more info
about the architecture of this client.

**cli**

A simple cli client.

For example, to fetch the second page and show the cheapest results first:

```sh
./bin/cli -query "camera" -page 2 -sort price
```

Use `-sort price-desc` for the most expensive results first, or add a
postcode with `-sort distance` to order by the nearest store:

```sh
./bin/cli -query "camera" -postcode "SW1A 0AA" -sort distance
```

CLI pages are numbered 1 to 1000. Run `./bin/cli -h` or the Windows
alternative for all switches, or see the [CLI README](cmd/cli/README.md).

<img width="1000" src="cmd/cli/cli.gif" />

## Go module

`Search` continues to return the first page. Use `SearchPage` to request a
specific page and find out whether another page is available. Its page
argument is zero-based (`0` is the first page; valid values are `0` to
`999`). Use `SearchPageSorted` when pagination and sorting must be combined;
price ordering is applied upstream before selecting the requested page:

```go
package main

import (
    "fmt"
    "log"

    "github.com/rorycl/cexfind"
)

func main() {
    finder, err := cexfind.NewCexFind()
    if err != nil {
        log.Fatal(err)
    }
    boxes, hasMore, err := finder.SearchPageSorted(
        []string{"camera"}, false, "", 1, cexfind.SortPrice,
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%d results; more pages: %t\n", len(boxes), hasMore)
}
```

The sort constants are `SortModel`, `SortPrice`, `SortPriceDesc`, and
`SortDistance`. To sort by distance, initialize store distances with
`WithStoreDistanceInitiliase()` and pass a postcode to `SearchPage`.

## AI assistance

AI tools were used to add pagination, sorting, and minimum/maximum price
filtering across the web, console, and CLI clients.

## Licence

This project is licensed under the [MIT Licence](LICENCE).
