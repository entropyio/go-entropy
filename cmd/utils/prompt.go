// Package utils contains internal helper functions for go-entropy commands.
package utils

import (
	"fmt"
	"github.com/entropyio/go-entropy/cmd/console/prompt"
)

// GetPassPhrase displays the given text(prompt) to the user and requests some textual
// data to be entered, but one which must not be echoed out into the terminal.
// The method returns the input provided by the user.
func GetPassPhrase(text string, confirmation bool) string {
	if text != "" {
		fmt.Println(text)
	}
	password, err := prompt.Stdin.PromptPassword("Password: ")
	if err != nil {
		Fatalf("Failed to read password: %v", err)
	}
	if confirmation {
		confirm, err := prompt.Stdin.PromptPassword("Repeat password: ")
		if err != nil {
			Fatalf("Failed to read password confirmation: %v", err)
		}
		if password != confirm {
			Fatalf("Passwords do not match")
		}
	}
	return password
}

// GetPassPhraseWithList retrieves the password associated with an account, either fetched
// from a list of preloaded passphrases, or requested interactively from the user.
func GetPassPhraseWithList(text string, confirmation bool, index int, passwords []string) string {
	// If a list of passwords was supplied, retrieve from them
	if len(passwords) > 0 {
		if index < len(passwords) {
			return passwords[index]
		}
		return passwords[len(passwords)-1]
	}
	// Otherwise prompt the user for the password
	password := GetPassPhrase(text, confirmation)
	return password
}