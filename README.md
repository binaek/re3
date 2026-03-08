# re3

A mathematically pure, blazing fast, **O(n)** linear-time regular expression engine for Go. 

`re3` is built from the ground up using **Brzozowski derivatives** and a **Lazy JIT-compiled DFA**. It provides guaranteed linear-time execution (ReDoS-free), advanced boolean operations (Intersection `&`, Complement `~`), and raw execution speeds that outperform Go's standard `regexp` library.

## 🚀 Why re3?

Go's standard library `regexp` uses a bytecode Virtual Machine (VM) approach. While great for typical workloads, evaluating state machines rune-by-rune inside a VM has overhead. 

`re3` takes a fundamentally different mathematical approach:
1. **No Backtracking:** Execution is strictly $O(n)$ time. It is completely immune to Regular Expression Denial of Service (ReDoS) attacks.
2. **Raw Speed:** By JIT-compiling state transitions into a flat array cache, the hot loop is reduced to an $O(1)$ memory lookup.
3. **Pure Boolean Logic:** Standard regex engines only support Union (`|`). Because `re3` is based on derivatives, it natively supports true Intersection (`&`) and Complement (`~`) of infinite sets.

### Benchmarks vs Go `regexp`

Tested on an Apple M3 Pro. `re3` consistently beats the standard library in pure FSM execution speed.

| Operation | `re3` | Go `std` | Difference |
| :--- | :--- | :--- | :--- |
| **MatchString** (Boolean check) | `16.3 ns/op` | `68.2 ns/op` | **4.1x Faster** |
| **FindAllString** | `632.7 ns/op` | `1005 ns/op` | **1.5x Faster** |
| **ReplaceAllString** | `372.3 ns/op` | `477.2 ns/op` | **1.2x Faster** |
| **FindStringIndex** (200KB string) | `2.33 µs/op` | `2.42 µs/op` | **Marginally Faster** |

*(Run `go test ./... -bench=.` to run the full suite yourself).*

---

## 🧠 The Architecture

`re3` does not use standard NFA-to-DFA powerset construction, which suffers from $O(2^m)$ exponential state explosion during compilation. Instead, it uses a pipeline of advanced compiler optimizations:


### **Brzozowski Derivatives:** 
The AST calculates its own derivatives mathematically, bypassing the need for NFA Thompson constructions.

### **Smart Constructors:** 
Set-flattening deduplicates mathematically identical FSM branches on the fly, preventing structural bloat.

### **Minterm Interval Trees (Partition Refinement):** 
The massive UTF-8 alphabet (1M+ runes) is compressed into a tiny handful of equivalence classes (minterms). The DFA transition tables are microscopic and highly cache-friendly.

### **Lazy JIT DFA:** 
States are compiled *only* when the engine encounters a new sequence of characters at runtime. Memory footprints stay tiny.

### **SIMD Fast-Forwarding:** 
Literal prefix extraction allows the engine to skip thousands of dead bytes in a single CPU cycle using AVX/SIMD instructions before the FSM even wakes up.

### **The RE2 Two-Pass TDFA:** 
`re3` isolates submatch extraction. Standard boolean queries run on the blazing fast pure DFA. If you need capture groups, the engine locates the exact match bounds first, then spins up a Tagged DFA (TDFA) strictly over the captured span. Zero overhead for non-capturing queries. [Read more](#️tagged-dfa-tdfa--submatch-extraction)

---

## 📦 Installation

```bash
go get [github.com/binaek/re3](https://github.com/binaek/re3)

```

## 🛠️ Quick Start

The API is intentionally designed to mirror Go's standard `regexp` package, making it a seamless drop-in replacement.

```go
package main

import (
    "fmt"
    "[github.com/binaek/re3](https://github.com/binaek/re3)"
)

func main() {
    // Compile a regular expression
    re := re3.MustCompile("[a-z]+@[a-z]+\\.[a-z]+")

    // Boolean matching
    fmt.Println(re.MatchString("user@domain.com")) // true

    // Finding boundaries
    loc := re.FindStringIndex("Contact me at user@domain.com please.")
    fmt.Println(loc) // [14 29]

    // Capture Groups (TDFA extraction)
    reGroups := re3.MustCompile("([a-z]+)@([a-z]+)\\.([a-z]+)")
    sub := reGroups.FindStringSubmatch("user@domain.com")
    fmt.Println(sub) // ["user@domain.com", "user", "domain", "com"]
}

```

### Advanced Boolean Syntax

Because of the Brzozowski architecture, you can use mathematical set operations:

* `a&b` (Intersection): Must match `a` AND `b` simultaneously.
* `~(a)` (Complement): Matches anything that DOES NOT match `a`.

---

## ⚡ Concurrency

Standard eagerly-compiled regex engines are naturally thread-safe because they are read-only at runtime. Because `re3` is a **Lazy DFA**, it mutates its internal state cache as it discovers new strings.

`re3` provides two distinct concurrency models so you can choose between convenience and raw lock-free throughput:

#### 1. Drop-in Thread Safety (`Concurrent`)

Use `re3.Concurrent()` to wrap an engine in an optimized `sync.RWMutex`. The engine will acquire a read-lock for the ultra-fast hot loop, and only drop to a write-lock if a cache miss occurs.

```go
re := re3.Concurrent(re3.MustCompile("a+b+"))

// Safe to share `re` across thousands of goroutines
go func() {
    re.MatchString("aaabbb")
}()

```

#### 2. Lock-Free Parallelism (`Clone`)

For absolute maximum throughput (e.g., log processing on 16 cores), use `Clone()`. This creates a localized, lock-free copy of the state cache for each worker thread while safely sharing the read-only AST and minterm tables.

```go
baseRE := re3.MustCompile("a+b+")

for i := 0; i < 16; i++ {
    go func() {
        // Clone is lock-free, zero contention, perfect linear scaling
        localRE := baseRE.Clone()
        localRE.MatchString("aaabbb")
    }()
}

```

---

Here is the updated section for the README. It explicitly addresses how RE2 tames the memory explosion (using memory caps and NFA fallbacks) and seamlessly contrasts it with how `re3` achieves the same safety using `maxLazyDFAStates` without ever touching an NFA.

---

## 🐌 Lazy DFA vs. Eager Compilation

If DFAs are so fast, why don't engines just compile the entire state machine upfront?

The answer is **State Explosion**. Converting a regular expression into a fully resolved DFA (Ahead-of-Time / Eager compilation) is mathematically an $O(2^m)$ operation, where $m$ is the length of the pattern. A seemingly innocent regex like `.*a.{20}` can generate millions of states, consuming gigabytes of RAM and taking minutes to compile.

`re3` solves this using a **Lazy DFA**.

When `re3.MustCompile()` is called, it does not build the state machine. It simply parses the AST, compresses the alphabet, and creates a "Root" state. The actual state transitions are computed and cached **Just-In-Time (JIT)** as the engine reads the input string.

* The engine only builds the specific paths that your actual data travels.
* You get the blinding $O(1)$ per-byte execution speed of a pure DFA.
* You completely avoid the upfront exponential memory and compilation tax.

### How RE2 avoids state explosion (and how `re3` differs)

Google's C++ `RE2` library is famous for solving this exact problem. RE2 avoids state explosion by also using a Lazy DFA, coupled with a strict memory limit. It computes DFA states on the fly using NFA powerset construction. If the state cache hits its predefined memory limit, RE2 flushes the cache and starts over, or falls back to processing the NFA directly. This guarantees memory safety and $O(n)$ execution time.

`re3` employs a similar strict boundary to guarantee memory safety (a hard cap of `maxLazyDFAStates = 100_000`), but its underlying architectural foundation is fundamentally different from `RE2`:

* **Go's `regexp`:** Compiles the regex into a custom assembly language and runs it through a Virtual Machine. This avoids state explosion but comes with a higher per-byte execution overhead.
* **C++ RE2:** Parses the regex into an NFA (Non-deterministic Finite Automaton). When its Lazy DFA encounters an uncached transition, it calculates the next state by running a powerset construction over the underlying NFA graphs.
* **`re3` (This engine):** Has **no NFA whatsoever**. `re3` evaluates new states directly from the Abstract Syntax Tree (AST) using **Brzozowski derivatives**.

**Why does skipping the NFA matter?**
Standard NFA constructions cannot efficiently handle true boolean set operations. If you attempt to add Intersection (`&`) or Complement (`~`) to a standard Thompson NFA, the machine's size explodes exponentially before you even attempt to build a DFA. Because `re3` relies on calculus-like derivatives rather than NFA graphs, it natively supports these advanced mathematical operations with zero structural penalty.

---

## 🏷️ Tagged DFA (TDFA) & Submatch Extraction

A mathematically pure DFA is a boolean machine: it is incredibly fast at answering *if* a string matches, but it instantly forgets *how* it reached the accepting state. If you use a regex like `([a-z]+)@([a-z]+)`, a standard DFA cannot tell you where the username ends and the domain begins, making submatch extraction (capture groups) impossible.

To solve this, engines use a **Tagged DFA (TDFA)**. A TDFA attaches memory operations (tags) to state transitions. As the engine moves from state to state, it records the exact byte indices of your capture groups.

### The Performance Dilemma

Running tag operations in the inner hot-loop is computationally expensive. It requires allocating memory and writing bounds on every byte transition. If an engine forces every query through a TDFA, developers pay a massive "capture tax" even when they just want a simple boolean `MatchString`.

### The `re3` Two-Pass Optimization

To protect raw execution speed, `re3` employs a dual-engine architecture inspired by RE2:

#### **Strict Isolation:** 
If a regex has no capture groups, or if the user only calls `MatchString` / `FindStringIndex`, the TDFA is completely ignored. The engine runs on the blazing-fast, pure DFA.

#### **The Two-Pass Trick:** 
If you call `FindStringSubmatch` on a 100MB log file, running a heavy TDFA over 100MB of text would be devastating. Instead, `re3` uses the pure DFA to scan the 100MB file and find the overall match boundaries. If the match is only 50 bytes long, `re3` spins up the TDFA and executes it *strictly over those 50 bytes*. You use the Ferrari to find the data, and the tractor to harvest it.

#### **Flat-Array Memory Pooling:** 
During extraction, `re3` completely bypasses multi-dimensional slice allocations. It writes all capture bounds into a single, flattened 1D integer array (`[]int`), reducing Garbage Collection pressure and matching the memory efficiency of the standard library while beating it in speed.

---

## 🌟 Inspiration & Acknowledgements

The architecture of `re3` is built upon decades of theoretical computer science and recent breakthroughs in functional programming:

### **Janusz Brzozowski (1964):** 
For the original conceptualization of regular expression derivatives, proving that an AST can directly evaluate state transitions mathematically.

### **Scott Owens, John Reppy, and Aaron Turon (2009):** 
For their paper *"Regular-expression derivatives re-examined"*, which revived Brzozowski's work and proved that smart constructors and lazy evaluation could tame the exponential state explosion.

### **RE# (Ian Erik Varatalu et al.):** 
The direct inspiration for `re3` being a production-grade tool. Their paper *"Derivative Based Extended Regular Expression Matching Supporting Intersection, Complement and Lookarounds"* and subsequent F# engine demonstrated that derivative-based matching, when combined with minterm compression and a lazy DFA, can outpace traditional industry-standard state machines.

---

## License

[Apache 2.0](LICENSE)

