# Known disagreements

Cases where the copies of the quarantine rules do not agree. Each names the
verdict this corpus holds to be right in `expected.json`, and the diverging
copy's actual behaviour under `diverges`. Resolving one means fixing that copy
and deleting its `diverges` entry and its section here, in the same change.

The Go copy is `hack/lib/testresults.go` in this repository; the Python copy
is `hack/lib/quarantine.py`, which the k3sm project tooling carries.

## event-empty-action

A package event `{"Action":"","Package":"..."}`.

- Expected: `red`. An event with an empty Action is malformed, and a malformed
  stream fails closed.
- Go: `red` ("event without Package/Action").
- Python: `green`. `results_from_go_json` checks only that Action is a string,
  so the empty string passes and the package-level event is ignored.

## registry-lone-cr

Two registry entries separated by a lone carriage return.

- Expected: `red`. The registry is line-delimited by LF, with CRLF tolerated,
  so the two entries are one line carrying a stray test id, which is malformed.
- Go: `red` (splits on `\n` after normalising `\r\n`; the stray id is "not
  key=value").
- Python: `green`. `parse_registry` uses `str.splitlines`, which also breaks
  lines on `\r`, `\v`, `\f`, `\x1c` to `\x1e`, `\x85`, ` ` and ` `.

## output-not-a-string

An output event whose `Output` is a number.

- Expected: `red` with a stream error, and the other results still reported.
- Go: `red` ("not JSON": the event does not decode into its string field).
- Python: raises `TypeError` while concatenating the output. Its CLI exits 1
  with a traceback rather than a report; the corpus records this as `crash`.
