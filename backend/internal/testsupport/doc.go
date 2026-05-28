// Package testsupport provides shared helpers for integration tests (an
// ephemeral Postgres container + per-test isolated databases). The real
// implementation lives in files behind the "integration" build tag; this file
// keeps the package non-empty for the default build.
package testsupport
