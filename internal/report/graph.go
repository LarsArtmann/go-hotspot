package report

import (
	"fmt"
	"io"

	"github.com/larsartmann/go-hotspot/internal/errors"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
	"github.com/larsartmann/go-output"
	"github.com/larsartmann/go-output/d2"
	"github.com/larsartmann/go-output/graph"
)

// couplingGraph builds an immutable go-output Graph from temporal coupling
// pairs. Each unique file becomes a node; each coupling pair becomes an edge
// labeled with the coupling degree percentage and shared commit count.
func couplingGraph(pairs []hotspot.CouplingPair) output.Graph {
	b := output.NewGraphBuilder()

	seen := make(map[string]struct{})
	ensureNode := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		b.AddNode(output.GraphNode{
			ID:    output.NewBrandedID[output.GraphNodeIDBrand](path),
			Label: output.NewBrandedID[output.GraphNodeLabelBrand](path),
			Shape: output.NodeShapeBox,
		})
	}

	for _, p := range pairs {
		ensureNode(p.FileA)
		ensureNode(p.FileB)

		b.AddEdge(output.GraphEdge{
			From: output.NewBrandedID[output.GraphNodeIDBrand](p.FileA),
			To:   output.NewBrandedID[output.GraphNodeIDBrand](p.FileB),
			Label: output.NewBrandedID[output.GraphNodeLabelBrand](
				fmt.Sprintf("%.0f%% (%d)", p.Degree, p.SharedCommits),
			),
		})
	}

	return b.Build()
}

func renderCouplingDOT(w io.Writer, pairs []hotspot.CouplingPair) error {
	if len(pairs) == 0 {
		return nil
	}

	g := couplingGraph(pairs)
	if err := graph.WriteDOT(w, g,
		graph.WithDirected(false),
		graph.WithGraphID("coupling"),
		graph.WithDOTRankDir(graph.RankDirLR),
	); err != nil {
		return errors.ReportRender("render DOT coupling graph", err)
	}

	return nil
}

func renderCouplingMermaid(w io.Writer, pairs []hotspot.CouplingPair) error {
	if len(pairs) == 0 {
		return nil
	}

	g := couplingGraph(pairs)
	if err := graph.WriteMermaid(w, g, graph.WithCodeFence(false)); err != nil {
		return errors.ReportRender("render Mermaid coupling graph", err)
	}

	return nil
}

func renderCouplingD2(w io.Writer, pairs []hotspot.CouplingPair) error {
	if len(pairs) == 0 {
		return nil
	}

	g := couplingGraph(pairs)
	if err := d2.WriteGraph(w, g, d2.WithDirection(d2.DirRight)); err != nil {
		return errors.ReportRender("render D2 coupling graph", err)
	}

	return nil
}
