package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	cex "github.com/rorycl/cexfind"
)

func TestSortSelection(t *testing.T) {
	in := newInModel()
	in.cursor = cursorSort
	for _, want := range []string{cex.SortPrice, cex.SortPriceDesc, cex.SortModel} {
		updated, _ := in.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		in = updated.(inModel)
		if in.sortBy != want {
			t.Fatalf("sort without postcode: got %q, want %q", in.sortBy, want)
		}
	}
	in.postcode.SetValue("SW1A 0AA")
	for _, want := range []string{cex.SortPrice, cex.SortPriceDesc, cex.SortDistance} {
		updated, _ := in.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		in = updated.(inModel)
		if in.sortBy != want {
			t.Fatalf("sort with postcode: got %q, want %q", in.sortBy, want)
		}
	}
	in.postcode.SetValue("")
	updated, _ := in.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	in = updated.(inModel)
	if in.sortBy != cex.SortModel {
		t.Errorf("clearing postcode left distance selected: %q", in.sortBy)
	}
}

func TestSortFocus(t *testing.T) {
	m := model{input: newInModel(), list: newLiModel(), state: checkboxState, status: newSelection()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.state != sortState || m.input.cursor != cursorSort {
		t.Fatalf("Tab from strict focused %s, cursor %d", m.state, m.input.cursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.state != inputState {
		t.Errorf("Tab from sort focused %s, want input", m.state)
	}
}

func TestFindPriceSort(t *testing.T) {
	contents, err := os.ReadFile("../../testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, string(contents))
	}))
	defer ts.Close()
	oldURL := cex.URL
	cex.URL = ts.URL
	defer func() { cex.URL = oldURL }()
	finder, err := cex.NewCexFind()
	if err != nil {
		t.Fatal(err)
	}
	m := model{cex: finder, input: newInModel()}
	m.input.sortBy = cex.SortPriceDesc
	items, count, _, err := find(&m, "lenovo x390", false, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 6 || len(items) < 2 {
		t.Fatalf("got %d results and %d list entries", count, len(items))
	}
	first := items[1].(item)
	if !strings.Contains(first.title, "£360") {
		t.Errorf("highest price was not first: %q", first.title)
	}
}
