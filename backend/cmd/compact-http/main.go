package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/compaction"
	"github.com/stillmind/backend/internal/config"
	"github.com/stillmind/backend/internal/memory"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	store, err := memory.Open(ctx, cfg.CRDBURL, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("crdb: %v", err)
	}
	defer store.Close()

	bc, err := bedrock.New(ctx, cfg.AWSRegion, cfg.BedrockModel, cfg.BedrockEmbed, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	w := &compaction.Worker{
		Store: store, Bedrock: bc,
		Budget: cfg.CompactBudgetTokens, Pressure: cfg.CompactPressureThreshold,
		MinS: cfg.CompactMinIntervalS, Threshold: 0.9,
	}
	uid := cfg.UserID()
	last := time.Time{}

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		utts, err := w.LoadRecent(r.Context(), uid, 200)
		if err != nil {
			log.Printf("load: %v", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		dec, err := w.MaybeCompact(r.Context(), uid, utts, last)
		if err != nil {
			log.Printf("compact: %v", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		if dec.Fired {
			log.Printf("compact fired ratio=%.2f score=%.2f", dec.Ratio, dec.Score)
			last = time.Now()
		} else {
			log.Printf("compact skipped: %s", dec.Reason)
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Write([]byte(`{"fired":` + boolStr(dec.Fired) + `,"reason":"` + dec.Reason + `"}`))
	})

	log.Printf("compact-http listening on :%s user=%s", cfg.Port, uid)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
