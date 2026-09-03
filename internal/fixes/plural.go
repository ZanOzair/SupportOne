package fixes

// PluralKey picks the singular variant of a message key when the count is one.
//
// It mirrors checks.PluralKey deliberately rather than sharing it: both
// packages name message keys without depending on the catalog that resolves
// them, and one small function is a smaller price than that dependency.
func PluralKey(base string, n int) string {
	if n == 1 {
		return base + ".one"
	}
	return base
}
