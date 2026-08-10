package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/larsartmann/go-hotspot/internal/git"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

func main() {
	now := time.Now()

	history, err := git.Collect(context.Background(), git.Options{
		Since: "1 year ago",
	}, now)
	if err != nil {
		log.Fatal(err)
	}

	pairs := hotspot.Coupling(history, hotspot.DefaultCouplingOptions())
	for _, p := range pairs {
		fmt.Printf("%-30s ↔ %-30s   shared=%d  degree=%.0f%%\n",
			p.FileA, p.FileB, p.SharedCommits, p.Degree)
	}
}
