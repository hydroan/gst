// Package testsupport is what the test packages share: the password of the
// seeded accounts the tests log in as.
package testsupport

// Password is the password every seeded account starts with in the tests:
// TestMain hands it to the startup hook as SEED_PASSWORD and to the iam
// module, for the root account, as IAM_ROOT_PASSWORD.
const Password = "12345678"
