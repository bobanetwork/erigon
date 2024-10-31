This document is an attempt to outline the procedure for merging in changes
from upstream.

Merge conflicts in this repository tend to be substantive when they occur, so
this document is currently fairly limited.  However, the intent is to have it
here as a place to document any repetitive merge techniques that may not be
initially obvious.

### Typical merge conflicts

### `go.mod`/`go.sum`

Generally, the safest way to resolve this conflicts is to remove duplicate
dependencies between the two sections, leaving the one at the higher version.
Once done, simply run `go mod tidy`.  Note, the tidy step will likely fail
until any `*.go` conflicts have been resolved.

Note, we have replace directives in most go.mod files that must be kept up to
date as well.

### `*.pb.go`

If any of the generated protobuf files have conflicts at merge, then most
likely something has changed upstream in the `interfaces` repository.  First,
merge this upstream repo to match the version referenced, and update the
`replace` directive in the `go.mod`.  Finally regenerate the protos by running
`make` in the `erigon-lib` directory.

### Flags

We have additional flags for historical RPC endpoints.  These are often the
subject of merge conflicts, and need only be preserved in addition to any
other flag modifications.
