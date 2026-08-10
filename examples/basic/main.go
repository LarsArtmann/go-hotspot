package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	"github.com/larsartmann/go-hotspot/internal/git"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

func main() {
	now := time.Now()

	history, err := git.Collect(context.Background(), git.Options{
		Since:       "1 year ago",
		HalfLifeDay: 180,
	}, now)
	if err != nil {
		log.Fatal(err)
	}

	complexities := make(map[string]complexity.FileComplexity)
	for path := range history.Files {
		fc, err := complexity.Analyze(path)
		if err != nil {
			log.Printf("warning: skipping %s: %v", path, err)
			continue
		}

		complexities[path] = fc
	}

	results := hotspot.Score(history, complexities, hotspot.ScoreOptions{
		Complexity: hotspot.MetricCyclomatic,
		Churn:      hotspot.ChurnWeighted,
	})
	hotspot.Sort(results, hotspot.SortHotspot, now)

	for _, r := range results[:min(5, len(results))] {
		fmt.Printf("%-40s  commits=%d  cyc=%d  hotspot=%.6f\n",
			r.Path, r.Commits, r.Cyclomatic, r.Hotspot)
	}
}
