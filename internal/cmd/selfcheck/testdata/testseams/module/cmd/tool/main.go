package main

// Exported is in a main package: not judged.
func Exported() int { return 1 }

func main() { _ = Exported() }
