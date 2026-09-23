package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPriceInputs(t *testing.T) {
	m := model{input: newInModel(), state: inputState, keys: getKeyMap(inputKeysState)}
	for _, want := range []state{postcodeState, minPriceState, maxPriceState, checkboxState} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = updated.(model)
		if m.state != want {
			t.Fatalf("tab reached %s, want %s", m.state, want)
		}
	}
	m.input.minPrice.SetValue("25.5")
	m.input.maxPrice.SetValue("100")
	updated, cmd := m.Update(inputEnterMsg("test item"))
	m = updated.(model)
	if cmd == nil || m.price.Min == nil || m.price.Max == nil || m.price.Min.String() != "25.5" || m.price.Max.String() != "100" {
		t.Errorf("valid price input did not start search: price=%+v cmd=%v", m.price, cmd)
	}
	m.input.minPrice.SetValue("101")
	updated, cmd = m.Update(inputEnterMsg("test item"))
	m = updated.(model)
	if cmd != nil || !strings.Contains(string(m.status), "min price must not exceed max price") {
		t.Errorf("invalid range started search: status=%q cmd=%v", m.status, cmd)
	}
}

func TestSearchPageKeys(t *testing.T) {
	m := model{state: listState, page: 1, hasMore: true, query: "test", strict: true, postcode: "SW1A 0AA"}
	for _, tc := range []struct {
		key      rune
		wantPage int
	}{{']', 2}, {'[', 0}} {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
		if cmd == nil {
			t.Fatalf("key %q did not fetch a page", tc.key)
		}
		msg, ok := cmd().(findPerformMsg)
		if !ok || msg.page != tc.wantPage || msg.query != m.query || !msg.strict || msg.postcode != m.postcode {
			t.Errorf("key %q produced %#v", tc.key, msg)
		}
	}
}

func TestToggleAllStores(t *testing.T) {
	m := model{state: listState, list: newLiModel()}
	m.list.list.SetSize(100, 10)
	m.list.ReplaceList([]list.Item{item{
		title: "Test item", description: "     (£10/£20) London, Bristol",
		noStoresDescription: "     (£10/£20)",
	}})
	if !strings.Contains(m.list.View(), "London, Bristol") {
		t.Fatal("store names not shown initially")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(model)
	if !m.list.storesCollapsed || strings.Contains(m.list.View(), "London") || !strings.Contains(m.list.View(), "£10/£20") {
		t.Fatalf("collapsed result did not retain prices: %q", m.list.View())
	}
	m.list.ReplaceList([]list.Item{item{
		title: "Next page item", description: "     (£30/£40) York",
		noStoresDescription: "     (£30/£40)",
	}})
	if strings.Contains(m.list.View(), "York") {
		t.Fatal("store names reappeared on the next page")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(model)
	if m.list.storesCollapsed || !strings.Contains(m.list.View(), "York") {
		t.Fatalf("store names not restored: %q", m.list.View())
	}
}

func TestMain(t *testing.T) {

	// does not add empty items between headings as done by "find" (see
	// find.go)
	items := []list.Item{
		item{title: "this is a heading", isHeading: true},
		item{title: "this is a normal item 1", url: "https://test.com/abc/a"},
		item{title: "this is a normal item 2", url: "https://test.com/abc/b"},
		item{title: "this is a normal item 3 ... and some more text", url: "https://test.com/abc/c"},
		item{title: "this is another heading", isHeading: true},
		item{title: "this is a normal item 4", url: "https://test.com/abc/d"},
		item{title: "this is a normal item 5", url: "https://test.com/abc/e"},
		item{title: "this is a heading b", isHeading: true},
		item{title: "b this is a normal item 1", url: "https://test.com/abc/f"},
		item{title: "b this is a normal item 2", url: "https://test.com/abc/g"},
		item{title: "b this is a normal item 3 this is a normal item 3b this is a normal ...", url: "https://test.com/abc/h"},
		item{title: "this is another heading c", isHeading: true},
		item{title: "c this is a normal item 4", url: "https://test.com/abc/i"},
		item{title: "c this is a normal item 5", url: "https://test.com/abc/j"},
		item{title: "this is a heading d", isHeading: true},
		item{title: "d this is a normal item 1", url: "https://test.com/abc/k"},
		item{title: "d this is a normal item 2", url: "https://test.com/abc/l"},
		item{title: "d this is a normal item 3 this is a normal item 3.", url: "https://test.com/abc/m"},
		item{title: "this is another heading e", isHeading: true},
		item{title: "e this is a normal item 4", url: "https://test.com/abc/n"},
		item{title: "e this is a normal item 5", url: "https://test.com/abc/o"},
	}

	m, err := NewModel("")
	if err != nil {
		t.Fatal(err)
	}
	m.list.ReplaceList(items)

	if got, want := len(m.list.list.Items()), 21; got != want {
		t.Errorf("list length got %d want %d", got, want)
	}
	if got, want := m.input.cursor, cursorInput; got != want {
		t.Errorf("input cursor got %d want %d", got, want)
	}
	if got, want := m.input.checkbox, false; got != want {
		t.Errorf("checkbox set to %t want %t", got, want)
	}

}

func TestNewModelFailures(t *testing.T) {

	tests := []struct {
		proxy string
		ok    bool
	}{
		{"socks5://127.0.0.1:8081", true},
		{"socks7://127.0.0.1:8081", false},
		{"nonsense", false},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			_, err := NewModel(tt.proxy)
			if err != nil && tt.ok {
				t.Errorf("error %s, expected none", err)
			}
			if err == nil && !tt.ok {
				t.Errorf("go no error, expected one")
			}
		})
	}
}
