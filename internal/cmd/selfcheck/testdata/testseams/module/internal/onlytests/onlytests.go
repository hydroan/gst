package onlytests

// Exported is used by a test of another package alone, and so is every
// export of this file.
func Exported() int { return 1 }

// Flag is used by this package's own test alone.
var Flag = 1
