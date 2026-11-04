// Package github stands in for the github package (the analyzer exempts only its own constructors there); never import it.
package github

import "net/http"

func GhBaseURL() string { return "https://api.github.com/" }

// Violation builds a GitHub request with raw http.NewRequest.
func Violation() (*http.Request, error) { return http.NewRequest("GET", GhBaseURL()+"user", nil) }
