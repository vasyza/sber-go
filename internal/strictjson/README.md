# Strict JSON boundary

`Validate(data []byte) error` validates **one complete byte buffer**, not a stream. It leaves the supplied bytes unchanged, including on rejection. All failures return static `ErrInvalidJSON`; raw IDs, amounts or source syntax are not inserted into diagnostics.

Requirements enforced before a permissive decode:
- valid UTF-8 and complete JSON grammar;
- paired Unicode surrogate escapes; literal U+FFFD, valid pairs and escaped-backslash text remain valid;
- unique **decoded** keys in every object; separate objects may use the same keys and canonical Unicode equivalence is not conflated;
- JSON number lexemes remain untouched; validation does not convert to binary float.

The package inherits `encoding/json`'s maximum nesting depth of **10000**. Depth 10001 is rejected. It intentionally has **no byte-size policy**: the transport/import caller must impose a source-appropriate input limit before allocating an unbounded buffer, then treat overflow/truncation as an error, never as a valid incomplete history. A stream adapter must pass the complete accepted document, not only an initial fragment.

`internal/strictjson/validate.go` passed independent frozen-snapshot review; this approval does not approve callers, bank parsing, session persistence, CLI or the complete SDK. Test-only strengthening after review adds explicit generated negative oracles, because a decode→marshal round trip erases duplicate properties and invalid surrogate spelling and is not sufficient proof of rejection.
