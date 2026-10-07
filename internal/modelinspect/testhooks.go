package modelinspect

// This file holds the seam the framework's own tests reach for, and nothing
// else. The tests of the gg command and of gg check's index rule run
// inspections in temporary projects, and each run leaves its result in the
// user cache directory, under a directory named after the project path; the
// tests remove it when they end. The package is internal, so nothing outside
// the framework can call it, and no path gg runs does: Inspect finds the
// directory on its own.

// CacheDir returns the directory holding the cached inspection results of the
// project in the working directory.
func CacheDir() (string, error) {
	return inspectionCacheDir()
}
