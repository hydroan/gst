// Package listing is the package whose tests the testlisting check reads.
package listing

import "os"

// Siblings lists the directory above the package: code outside tests is none
// of the check's business.
func Siblings() ([]os.DirEntry, error) {
	return os.ReadDir("..")
}
