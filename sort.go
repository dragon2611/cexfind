package cexfind

import (
	"cmp"
	"slices"
)

const (
	SortModel     = "model"
	SortPrice     = "price"
	SortPriceDesc = "price-desc"
	SortDistance  = "distance"
)

// SortBoxes orders search results in place. Distance uses the nearest store
// with a known location and places items with no known distance last.
func SortBoxes(results []Box, order string) {
	if order != SortPrice && order != SortPriceDesc && order != SortDistance {
		b := boxes(results)
		b.sort()
		return
	}
	slices.SortStableFunc(results, func(a, b Box) int {
		var comparison int
		switch order {
		case SortDistance:
			aDistance, aOK := nearestDistance(a)
			bDistance, bOK := nearestDistance(b)
			if aOK != bOK {
				if aOK {
					return -1
				}
				return 1
			}
			if aOK {
				comparison = cmp.Compare(aDistance, bDistance)
			}
		default:
			comparison = a.Price.Compare(b.Price)
			if order == SortPriceDesc {
				comparison = -comparison
			}
		}
		if comparison != 0 {
			return comparison
		}
		if comparison = cmp.Compare(a.Model, b.Model); comparison != 0 {
			return comparison
		}
		if comparison = a.Price.Compare(b.Price); comparison != 0 {
			return comparison
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func nearestDistance(box Box) (float64, bool) {
	var nearest float64
	found := false
	for _, store := range box.Stores {
		if store.StoreID != 0 && (!found || store.DistanceMiles < nearest) {
			nearest = store.DistanceMiles
			found = true
		}
	}
	return nearest, found
}
