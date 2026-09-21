package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Genzee/opsplatform/internal/adapters/tools"
	"github.com/Genzee/opsplatform/internal/fixture"
	"github.com/Genzee/opsplatform/internal/operations"
	"github.com/Genzee/opsplatform/internal/repository"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("fixture", "", "synthetic fixture JSON (required)")
	kind := flag.String("kind", "", "exact concrete kind filter")
	provider := flag.String("provider", "", "exact provider filter")
	namespace := flag.String("namespace", "", "Kubernetes namespace filter")
	stale := flag.Bool("include-stale", false, "include stale observations, with warnings")
	depth := flag.Int("max-depth", 16, "traversal depth limit")
	nodes := flag.Int("max-nodes", 1000, "traversal node limit")
	edges := flag.Int("max-edges", 5000, "traversal edge limit")
	direction := flag.String("direction", "", "neighbors/path direction: in, out, both")
	limit := flag.Int("limit", 100, "resource page size")
	offset := flag.Int("offset", 0, "resource page offset")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: infra -fixture FILE [flags] resources|get|neighbors|trace|dependencies|impact|path|tools [name-or-id] [target]\nAll flags precede the command. Output is JSON; fixture data is synthetic.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *path == "" || flag.NArg() == 0 {
		flag.Usage()
		return fmt.Errorf("fixture and command are required")
	}
	ctx := context.Background()
	store := repository.NewMemory()
	if err := fixture.Load(ctx, *path, store); err != nil {
		return err
	}
	service := operations.New(store)
	filter := operations.Filter{Kind: *kind, Provider: *provider, Namespace: *namespace, IncludeStale: *stale, Limit: *limit, Offset: *offset}
	command := flag.Arg(0)
	profiles := map[string]string{"neighbors": "neighbors", "trace": "infrastructure/v1", "dependencies": "dependencies/v1", "impact": "potential-impact/v1", "path": "path"}
	var result any
	var err error
	switch command {
	case "resources":
		if flag.NArg() != 1 {
			return fmt.Errorf("resources takes no positional arguments")
		}
		result, err = service.FindResources(ctx, filter)
	case "tools":
		if flag.NArg() != 1 {
			return fmt.Errorf("tools takes no positional arguments")
		}
		result = tools.New(service).Definitions()
	default:
		if command != "get" && profiles[command] == "" {
			return fmt.Errorf("unknown command %q", command)
		}
		want := 2
		if command == "path" {
			want = 3
		}
		if flag.NArg() != want {
			return fmt.Errorf("%s expects %d resource arguments", command, want-1)
		}
		id, resolveErr := service.Resolve(ctx, flag.Arg(1), filter)
		if resolveErr != nil {
			return resolveErr
		}
		if command == "get" {
			result, err = service.GetResource(ctx, id)
			break
		}
		q := operations.Traversal{ResourceID: id, Profile: profiles[command], IncludeStale: *stale, MaxDepth: *depth, MaxNodes: *nodes, MaxEdges: *edges, Direction: *direction}
		if command == "path" {
			q.TargetID, err = service.Resolve(ctx, flag.Arg(2), operations.Filter{})
			if err != nil {
				return err
			}
		}
		result, err = service.Traverse(ctx, q)
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
