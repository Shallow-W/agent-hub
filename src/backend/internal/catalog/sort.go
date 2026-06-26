package catalog

import "sort"

// sortItems is a stable-ish (relies on sort.Slice) wrapper used by the
// per-domain sorters in domains.go. It exists to keep the Sorter functions
// declarative and to centralize the ordering primitive so future domains
// reuse the same comparator shape.
func sortItems(items []Item, less func(a, b Item) bool) {
	if len(items) <= 1 {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		return less(items[i], items[j])
	})
}
