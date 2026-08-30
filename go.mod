module k3sm.io/apis

go 1.25.8

require (
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260618152121-87f3d3e198d3
	google.golang.org/grpc v1.81.1
	google.golang.org/protobuf v1.36.11
	k8s.io/apimachinery v0.35.0
)

require (
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912 // indirect
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

// genproto monolith-vs-split: grpc pulls the split googleapis/{api,rpc} modules,
// but the legacy monolith google.golang.org/genproto also satisfies some import
// paths — leaving the resolution ambiguous outside go.work. Pin the monolith to
// the same commit k3sm uses so `GOWORK=off go build` is deterministic (this
// replace is per-module; it does NOT cross from k3sm/go.mod into here). Keep in
// lockstep with k3sm/go.mod.
replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260622175928-b703f567277d
