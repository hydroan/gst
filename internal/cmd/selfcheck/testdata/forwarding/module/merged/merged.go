// Package merged has an exported function that only forwards to an unexported
// one nothing else uses.
package merged

// Record is a stored value.
type Record struct{ Size int }

// Total adds up the sizes of the records.
func Total(records []Record) int { return total(records) }

func total(records []Record) int {
	sum := 0
	for _, r := range records {
		sum += r.Size
	}
	return sum
}
