//go:build fixtureextra

package consumer

import "example.com/module/internal/used"

// Extra uses Tagged behind a build constraint.
func Extra() int { return used.Tagged() }
