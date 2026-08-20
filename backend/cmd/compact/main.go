package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/compaction"
	"github.com/stillmind/backend/internal/config"
	"github.com/stillmind/backend/internal/memory"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := memory.Open(ctx, cfg.CRDBURL, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("crdb: %v", err)
	}
	defer store.Close()

	bc, err := bedrock.New(ctx, cfg.AWSRegion, cfg.BedrockModel, cfg.BedrockEmbed, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("bedrock: %v", err)
	}

	w := &compaction.Worker{
		Store: store, Bedrock: bc,
		Budget: cfg.CompactBudgetTokens, Pressure: cfg.CompactPressureThreshold,
		MinS: cfg.CompactMinIntervalS, Threshold: 0.9,
	}

	uid := cfg.UserID()
	last := time.Time{}
	tick := time.NewTicker(2 * time.Minute)
	defer tick.Stop()

	log.Printf("compact worker started user=%s", uid)
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tick.C:
			utts, err := w.LoadRecent(ctx, uid, 200)
			if err != nil {
				log.Printf("load: %v", err)
				continue
			}
			dec, err := w.MaybeCompact(ctx, uid, utts, last)
			if err != nil {
				log.Printf("compact: %v", err)
				continue
			}
			if dec.Fired {
				log.Printf("compact fired ratio=%.2f score=%.2f", dec.Ratio, dec.Score)
				last = t
			} else {
				log.Printf("compact skipped: %s", dec.Reason)
			}
		}
	}
}
