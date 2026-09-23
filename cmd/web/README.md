# webserver

<img width="1000" src="./web.gif" />

The animated gif was made with Charm [vhs](https://github.com/charmbracelet/vhs).

Results can be sorted by model, price in either direction, or nearest store
when a postcode is entered and store distances are available.
Previous and Next fetch one page at a time, with up to 50 hits per query.
Sorting applies to the current page.

## Usage

```
Usage of ./webserver:
  -address string
    	server network address (default "127.0.0.1")
  -port string
    	server network port (default "8000")
  -proxy string
    	proxy, such as 'socks5://127.0.0.1:8081'

run a webserver to search Cex/Webuy for second hand equipment

eg <programme> -address 192.168.4.5 -port 8001
```
