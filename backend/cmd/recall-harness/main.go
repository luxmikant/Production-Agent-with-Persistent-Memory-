package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/config"
	"github.com/stillmind/backend/internal/memory"
)

type row struct {
	Body          string   `json:"body"`
	ExpectedTerms []string `json:"expected_terms"`
}

func main() {
	cfg := config.Load()
	ctx := context.Background()

	_ = os.Setenv("STILLMIND_STUB", "1")
	store, err := memory.Open(ctx, cfg.CRDBURL, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("crdb: %v", err)
	}
	defer store.Close()
	bc, _ := bedrock.New(ctx, cfg.AWSRegion, cfg.BedrockModel, cfg.BedrockEmbed, cfg.EmbedDim)

	b, err := os.ReadFile("benchmarks/recall/corpus.jsonl")
	if err != nil {
		log.Fatalf("corpus: %v", err)
	}
	uid := cfg.UserID()
	embeddings := map[string][]float32{}
	var rows []row
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r row
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		rows = append(rows, r)
		emb, err := bc.Embed(ctx, r.Body)
		if err != nil {
			continue
		}
		_, _ = store.InsertNote(ctx, uid, "note", r.Body, emb, "benchmark", 0)
		embeddings[r.Body] = emb
	}

	queries := []string{
		"what helps me sleep",
		"something for focus",
		"evening calm",
	}

	vecHits := 0
	lexHits := 0
	hybridHits := 0
	total := 0
	for _, q := range queries {
		emb, _ := bc.Embed(ctx, q)
		vec, _ := store.RetrieveByVector(ctx, uid, emb, 5)
		lex, _ := store.RetrieveByLexical(ctx, uid, q, 5)
		hyb := merge(vec, lex)
		for _, r := range rows {
			if !matches(q, r) {
				continue
			}
			total++
			if has(r, vec) {
				vecHits++
			}
			if has(r, lex) {
				lexHits++
			}
			if has(r, hyb) {
				hybridHits++
			}
		}
	}

	out := map[string]any{
		"vector_only":  rec(vecHits, total),
		"lexical_only": rec(lexHits, total),
		"hybrid":       rec(hybridHits, total),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func has(r row, items []memory.Retrieved) bool {
	for _, it := range items {
		if strings.EqualFold(strings.TrimSpace(it.Body), strings.TrimSpace(r.Body)) {
			return true
		}
	}
	return false
}

func merge(a, b []memory.Retrieved) []memory.Retrieved {
	seen := map[string]bool{}
	out := []memory.Retrieved{}
	for _, s := range [][]memory.Retrieved{a, b} {
		for _, r := range s {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

func matches(q string, r row) bool {
	ql := strings.ToLower(q)
	for _, t := range r.ExpectedTerms {
		if strings.Contains(ql, strings.ToLower(t)) {
			return true
		}
	}
	return false
}

func rec(h, t int) float64 {
	if t == 0 {
		return 0
	}
	v := float64(h) / float64(t)
	return float64(int(v*100)) / 100
}

// for go vet
var _ = fmt.Sprintf
