package columns

// This file holds the seam the framework's own tests reach for, and nothing
// else. The gg command's tests run gg gen in temporary projects, and each run
// leaves its column inspection result in the user cache directory, under a
// directory named after the project path; the tests remove it when they end.
// The package is internal, so nothing outside the framework can call it, and
// no path gg runs does: Generate finds the directory on its own.

// CacheDir returns the directory holding the cached column inspection results
// of the project in the working directory.
func CacheDir() (string, error) {
	return columnsCacheDir()
}
