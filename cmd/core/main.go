package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Genzee/opsplatform/internal/adapters/tools"
	"github.com/Genzee/opsplatform/internal/fixture"
	"github.com/Genzee/opsplatform/internal/operations"
	"github.com/Genzee/opsplatform/internal/repository"
)

func main() {
	path := flag.String("fixture", "", "synthetic fixture JSON to load (required)")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: core -fixture FILE < requests.jsonl")
		os.Exit(2)
	}
	ctx := context.Background()
	store := repository.NewMemory()
	if err := fixture.Load(ctx, *path, store); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := tools.New(operations.New(store)).Serve(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
