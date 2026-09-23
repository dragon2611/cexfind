# cexfind console app

This console app, which uses the
[cexfind](https://github.com/rorycl/cexfind/) go module to make searches
on Cex/Webuy, is written using the
[bubbletea](https://github.com/charmbracelet/bubbletea) TUI framework.

## Running the programme

This console app needs to be built or downloaded from the
[releases](https://github.com/rorycl/cexfind/releases) page. Please note
that the app only works for queries emanating from the UK.

SET the `PROXY` environmental variable to use, for example, a socks5
proxy.

Search results are fetched one upstream page at a time, with up to 50 hits
per query. While the result list is focused, press `]` for the next search
page or `[` for the previous one. The list's own scrolling stays separate.

Press Tab to reach the sort field, then Space or `x` to cycle through
model, price low to high, price high to low, and nearest store. Enter a
postcode to make nearest store available. Press Enter to search.
Sorting applies to the current search result page.
Tab through the minimum and maximum price fields to set an inclusive
selling price range in pounds. Leave either field blank for no bound.

![](console.gif)

This gif was made using Charm's
[vhs](https://github.com/charmbracelet/vhs):

```
vhs console.vhs
```

(I've been using `sxiv -af console.gif` or my browser to view the recording.)


## Structure

The app is structured around a main model which contains a list and
input model, together with a status area. The use of a custom list item
delegate is adapted from the example provided in the bubbletea
"list-fancy" example.

![](diagram.png)

The bubbletea repo includes two short but informative tutorials about
the bubbletea ELM architecture and commands.

I also found the Charm video on YouTube about the mini-project
[kancli](https://www.youtube.com/watch?v=ZA93qgdLUzM) helpful. The
`kancli` app is a simple cli kanban board with several models.

Commands (as explained in the tutorial/commands/README.md) are
particularly fun to use. For example the following `case` in the main
model's Update function is triggered on a `listEnterMsg`, which
temporarily sets the status area to notify the user of text having been
copied to the clipboard. This event itself triggers an asynchronous
update to reset the status area after 2.5s.

```go
// data was selected in the list view; reset the status after a
// short wait
case listEnterMsg:
	clipboard.Write(clipboard.FmtText, []byte(msg.url))
	m.status = m.status.setCopied(msg.String())
	return m, func() tea.Msg {
		time.Sleep(2500 * time.Millisecond)
		return resetListStatus{}
	}
```

### Files

The console component includes the following files:

```
README.md
go.mod
go.sum
main.go         # entry point
model.go        # the main model
status.go       # the status component (used in the model)
input.go        # the input sub-model
list.go         # the list sub-model
delegate.go     # the list item delegate
find.go         # the model's adapter to the main cexfind
keymap.go       # keymaps, used for interaction and help
model_test.go   # test
list_example.go # example list contents
console.vhs     # vhs instruction file
console.gif     # example gif
diagram.svg     # diagram of the program
diagram.png     # diagram png
```
