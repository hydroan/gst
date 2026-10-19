package seamed

// This file holds the seams the tests reach for.

// Hook is a seam the tests of other packages set.
var Hook = 1

// Leaked is declared among the seams but used by consumer.
func Leaked() int { return 1 }
