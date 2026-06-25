module k3sm.io/apis

go 1.25.8

require (
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260618152121-87f3d3e198d3
	google.golang.org/grpc v1.81.1
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

// genproto monolith-vs-split: grpc pulls the split googleapis/{api,rpc} modules,
// but the legacy monolith google.golang.org/genproto also satisfies some import
// paths — leaving the resolution ambiguous outside go.work. Pin the monolith to
// the same commit k3sm uses so `GOWORK=off go build` is deterministic (this
// replace is per-module; it does NOT cross from k3sm/go.mod into here). Keep in
// lockstep with k3sm/go.mod.
replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260622175928-b703f567277d
