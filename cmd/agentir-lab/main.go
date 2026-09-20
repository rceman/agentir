package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/project"
)

type sample struct {
	Path      string  `json:"path"`
	Function  string  `json:"function"`
	RawBytes  int     `json:"raw_bytes"`
	IRBytes   int     `json:"ir_bytes"`
	ByteRatio float64 `json:"byte_ratio"`
}

type report struct {
	Root          string   `json:"root"`
	Files         int      `json:"files"`
	Functions     int      `json:"functions"`
	Failures      int      `json:"failures"`
	RawBytes      int64    `json:"raw_bytes"`
	IRBytes       int64    `json:"ir_bytes"`
	WeightedRatio float64  `json:"weighted_ratio"`
	MedianRatio   float64  `json:"median_ratio"`
	P90Ratio      float64  `json:"p90_ratio"`
	P99Ratio      float64  `json:"p99_ratio"`
	MaxRatio      float64  `json:"max_ratio"`
	UnderOne      int      `json:"under_one"`
	OverTwo       int      `json:"over_two"`
	Worst         []sample `json:"worst"`
}

func main() {
	jsonOut := flag.Bool("json", false, "emit JSON")
	flag.Parse()
	root := filepath.Join(runtime.GOROOT(), "src")
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	r, err := measure(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentir-lab:", err)
		os.Exit(1)
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintln(os.Stderr, "agentir-lab:", err)
			os.Exit(1)
		}
		return
	}
	printReport(r)
}

func measure(root string) (report, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return report{}, err
	}
	r := report{Root: absRoot}
	var samples []sample

	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			r.Failures++
			return nil
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != absRoot && (name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			r.Failures++
			return nil
		}
		projection, err := project.Project(path, data)
		if err != nil {
			r.Failures++
			return nil
		}
		r.Files++
		for _, fn := range projection.Document.Functions {
			raw := fn.Source.EndOffset - fn.Source.StartOffset
			if raw <= 0 {
				continue
			}
			var b bytes.Buffer
			if err := ir.WriteCompactFunction(&b, fn); err != nil {
				r.Failures++
				continue
			}
			irBytes := b.Len()
			ratio := float64(irBytes) / float64(raw)
			rel, _ := filepath.Rel(absRoot, path)
			samples = append(samples, sample{
				Path:      filepath.ToSlash(rel),
				Function:  fn.Signature,
				RawBytes:  raw,
				IRBytes:   irBytes,
				ByteRatio: ratio,
			})
			r.Functions++
			r.RawBytes += int64(raw)
			r.IRBytes += int64(irBytes)
			if ratio < 1 {
				r.UnderOne++
			}
			if ratio > 2 {
				r.OverTwo++
			}
		}
		return nil
	})
	if err != nil {
		return report{}, err
	}
	if len(samples) == 0 {
		return r, nil
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i].ByteRatio < samples[j].ByteRatio })
	r.WeightedRatio = float64(r.IRBytes) / float64(r.RawBytes)
	r.MedianRatio = percentile(samples, 0.50)
	r.P90Ratio = percentile(samples, 0.90)
	r.P99Ratio = percentile(samples, 0.99)
	r.MaxRatio = samples[len(samples)-1].ByteRatio

	const worstCount = 10
	start := len(samples) - worstCount
	if start < 0 {
		start = 0
	}
	for i := len(samples) - 1; i >= start; i-- {
		r.Worst = append(r.Worst, samples[i])
	}
	return r, nil
}

func percentile(samples []sample, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	idx := int(float64(len(samples)-1) * p)
	return samples[idx].ByteRatio
}

func printReport(r report) {
	fmt.Printf("root: %s\n", r.Root)
	fmt.Printf("files: %d  functions: %d  failures: %d\n", r.Files, r.Functions, r.Failures)
	fmt.Printf("raw: %d B  agentir: %d B  weighted: %.3fx\n", r.RawBytes, r.IRBytes, r.WeightedRatio)
	fmt.Printf("median: %.3fx  p90: %.3fx  p99: %.3fx  max: %.3fx\n", r.MedianRatio, r.P90Ratio, r.P99Ratio, r.MaxRatio)
	fmt.Printf("functions <1.0x: %d  >2.0x: %d\n", r.UnderOne, r.OverTwo)
	fmt.Println("worst:")
	for _, s := range r.Worst {
		fmt.Printf("  %.3fx  %d -> %d  %s :: %s\n", s.ByteRatio, s.RawBytes, s.IRBytes, s.Path, s.Function)
	}
}
