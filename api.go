package re3

// MatchString reports whether the string s exactly matches the regular expression.
func (re *RegExp) MatchString(s string) bool {
	state := 0
	for i := 0; i < len(s); i++ {
		mintermID := re.minterms.ByteToClass[s[i]]
		state = re.forwardTransitions[state][mintermID]
	}
	return re.forwardIsMatch[state]
}

// Match reports whether the byte slice b exactly matches the regular expression.
func (re *RegExp) Match(b []byte) bool {
	state := 0
	for i := 0; i < len(b); i++ {
		mintermID := re.minterms.ByteToClass[b[i]]
		state = re.forwardTransitions[state][mintermID]
	}
	return re.forwardIsMatch[state]
}

// FindStringIndex returns the [start, end] of the leftmost-longest match in O(n) time.
func (re *RegExp) FindStringIndex(s string) []int {
	if len(s) == 0 {
		if re.forwardIsMatch[0] {
			return []int{0, 0}
		}
		return nil
	}

	// =========================================================
	// PHASE 1: UNANCHORED FORWARD SWEEP
	// Goal: Find the FIRST time the leftmost match accepts.
	// =========================================================
	firstEnd := -1
	state := 0
	for i := 0; i < len(s); i++ {
		mintermID := re.minterms.ByteToClass[s[i]]
		state = re.unanchoredTransitions[state][mintermID]

		if re.unanchoredIsMatch[state] {
			firstEnd = i + 1
			break // We found the earliest possible match, hit the brakes!
		}
	}

	if firstEnd == -1 {
		return nil // No match found anywhere in the string
	}

	// =========================================================
	// PHASE 2: ANCHORED REVERSE SWEEP
	// Goal: Run backwards to find the absolute leftmost start.
	// =========================================================
	revState := 0
	leftmostStart := -1

	for i := firstEnd - 1; i >= 0; i-- {
		mintermID := re.minterms.ByteToClass[s[i]]
		revState = re.reverseTransitions[revState][mintermID]

		if re.reverseIsMatch[revState] {
			leftmostStart = i // Keep overwriting to find the furthest left
		}
	}

	if leftmostStart == -1 && re.reverseIsMatch[0] {
		leftmostStart = firstEnd
	}

	// =========================================================
	// PHASE 3: ANCHORED FORWARD SWEEP
	// Goal: Run forward from the start to find the longest end.
	// =========================================================
	fwdState := 0
	longestEnd := -1

	if re.forwardIsMatch[0] {
		longestEnd = leftmostStart
	}

	for i := leftmostStart; i < len(s); i++ {
		mintermID := re.minterms.ByteToClass[s[i]]
		fwdState = re.forwardTransitions[fwdState][mintermID]

		if re.forwardIsMatch[fwdState] {
			longestEnd = i + 1 // Overwrite with the longest valid end
		}

		// Dead State Detection: If the state is not a match, and all minterms
		// loop back to this exact same state, the match is mathematically dead.
		isDead := !re.forwardIsMatch[fwdState]
		if isDead {
			for _, nextState := range re.forwardTransitions[fwdState] {
				if nextState != fwdState {
					isDead = false
					break
				}
			}
		}

		if isDead {
			break // Early exit guarantees O(n) performance
		}
	}

	return []int{leftmostStart, longestEnd}
}

// FindString returns a string holding the text of the leftmost-longest match.
func (re *RegExp) FindString(s string) string {
	loc := re.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

// FindAllString returns a slice of all successive matches of the expression.
// If n >= 0, the function returns at most n matches.
func (re *RegExp) FindAllString(s string, n int) []string {
	var matches []string
	pos := 0

	for pos <= len(s) && (n < 0 || len(matches) < n) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}

		start := pos + loc[0]
		end := pos + loc[1]
		matches = append(matches, s[start:end])

		// Advance the cursor. If the match was an empty string, we must advance
		// by 1 byte to prevent an infinite loop.
		if end == start {
			pos++
		} else {
			pos = end
		}
	}

	return matches
}
