package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

var updateGolden = flag.Bool("update-golden", false, "regenerate golden output files")

func TestGoldenAllFormats(t *testing.T) {
	results := sampleResults()
	summary := sampleSummary()
	couplings := []hotspot.CouplingPair{
		{FileA: "a.go", FileB: "b.go", SharedCommits: 15, Degree: 80},
	}

	cases := []struct {
		name      string
		format    Format
		couplings []hotspot.CouplingPair
	}{
		{"table", FormatTable, couplings},
		{"markdown", FormatMarkdown, couplings},
		{"csv", FormatCSV, nil},
		{"json", FormatJSON, couplings},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Render(&buf, results, tc.couplings, summary, tc.format, 0); err != nil {
				t.Fatal(err)
			}

			goldenPath := filepath.Join("testdata", "golden", tc.name+".txt")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(goldenPath, buf.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}

				return
			}

			expected, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden file %s: %v (run with -update-golden to create)", goldenPath, err)
			}

			if !bytes.Equal(buf.Bytes(), expected) {
				t.Errorf("%s output does not match golden file\n--- got ---\n%s\n--- want ---\n%s",
					tc.name, buf.String(), string(expected))
			}
		})
	}
}
