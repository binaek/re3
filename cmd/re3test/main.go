package main

import (
	"fmt"
	"log"

	"github.com/binaek/re3"
)

func main() {
	// The Regex: We want to extract paragraphs that DO NOT contain the word "ERROR".
	// - Starts with '['
	// - Contains a bunch of letters/spaces
	// - Ends with ']'
	// - MUST NOT contain the sequence "ERROR"
	regexStr := "(\\[[a-zA-Z ]+\\])&~(.*ERROR.*)"

	fmt.Printf("Compiling FSM for: %s\n\n", regexStr)

	re, err := re3.Compile(regexStr)
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}

	inputLog := `
[INFO system booted safely]
[WARN low memory]
[ERROR database connection failed]
[INFO retrying connection]
[ERROR retry failed]
[SUCCESS connected to backup]
`

	fmt.Println("--- Running FindAllString ---")
	fmt.Println("Goal: Extract all log brackets EXCEPT the ones containing ERROR.")

	// Extract all matches (n = -1)
	matches := re.FindAllString(inputLog, -1)

	if len(matches) == 0 {
		fmt.Println("❌ No matches found!")
	} else {
		for i, match := range matches {
			fmt.Printf("Match %d: %s\n", i+1, match)
		}
	}
	fmt.Println("\nExtraction Complete!")
}
