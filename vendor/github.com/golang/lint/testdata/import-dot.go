// Test that dot imports are flagged.

// Package pkg ...
package pkg

// MATCH /dot import/

var _ Stringer // from "fmt"
