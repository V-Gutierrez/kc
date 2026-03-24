package diff

import "sort"

type Status string

const (
	Added   Status = "+"
	Removed Status = "-"
	Changed Status = "~"
	Equal   Status = "="
)

type Entry struct {
	Key    string
	Status Status
	Left   string
	Right  string
}

func Compare(left, right map[string]string) []Entry {
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}

	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	entries := make([]Entry, 0, len(sorted))
	for _, key := range sorted {
		leftValue, leftOK := left[key]
		rightValue, rightOK := right[key]

		entry := Entry{Key: key, Left: leftValue, Right: rightValue}
		switch {
		case leftOK && !rightOK:
			entry.Status = Added
		case !leftOK && rightOK:
			entry.Status = Removed
		case leftValue != rightValue:
			entry.Status = Changed
		default:
			entry.Status = Equal
		}
		entries = append(entries, entry)
	}

	return entries
}
