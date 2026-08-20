package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/stillmind/backend/internal/agent"
	"github.com/stillmind/backend/internal/api"
	"github.com/stillmind/backend/internal/audit"
	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/compaction"
	"github.com/stillmind/backend/internal/config"
	"github.com/stillmind/backend/internal/memory"
	"github.com/stillmind/backend/internal/migrate"
	"github.com/stillmind/backend/internal/plan"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		dir := "migrations"
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "-dir" && i+1 < len(os.Args) {
				dir = os.Args[i+1]
			}
		}
		if err := migrate.Run(ctx, cfg.CRDBURL, dir); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		return
	}

	store, err := memory.Open(ctx, cfg.CRDBURL, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("crdb: %v", err)
	}
	defer store.Close()

	bc, err := bedrock.New(ctx, cfg.AWSRegion, cfg.BedrockModel, cfg.BedrockEmbed, cfg.EmbedDim)
	if err != nil {
		log.Printf("bedrock init: %v (running with stub if STILLMIND_STUB=1)", err)
	}

	au := &audit.Logger{Store: store}
	comp := &compaction.Worker{
		Store: store, Bedrock: bc,
		Budget: cfg.CompactBudgetTokens, Pressure: cfg.CompactPressureThreshold,
		MinS: cfg.CompactMinIntervalS, Threshold: 0.9,
	}
	loop := &agent.Loop{Store: store, Bedrock: bc, Compactor: comp, Audit: au}
	pb := &plan.Builder{Store: store}
	sh := &plan.MockSheets{Store: store}

	srv := &api.Server{
		Cfg: cfg, Store: store, Bedrock: bc, Loop: loop,
		Compact: comp, Plan: pb, Sheets: sh, Audit: au,
	}

	log.Printf("stillmind listening on :%s (user=%s)", cfg.Port, cfg.UserID())
	if err := srv.Start(ctx); err != nil && err.Error() != "http: Server closed" {
		log.Fatalf("server: %v", err)
	}
	os.Exit(0)
}
