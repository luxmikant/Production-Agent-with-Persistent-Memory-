package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/compaction"
	"github.com/stillmind/backend/internal/memory"
)

type Loop struct {
	Store     *memory.Store
	Bedrock   *bedrock.Client
	Compactor *compaction.Worker
	Audit     AuditLogger
}

type AuditLogger interface {
	Write(ctx context.Context, userID, typ string, payload any)
}

type TurnResult struct {
	Reply    string
	RunID    string
	Tokens   int
	Recall   []memory.Retrieved
	Compaction compaction.Decision
}

func (l *Loop) Step(ctx context.Context, userID, userMsg string) (TurnResult, error) {
	runID, err := l.Store.StartRun(ctx, userID)
	if err != nil {
		return TurnResult{}, err
	}

	emb, err := l.Bedrock.Embed(ctx, userMsg)
	if err != nil {
		return TurnResult{}, err
	}

	vecItems, err := l.Store.RetrieveByVector(ctx, userID, emb, 5)
	if err != nil {
		return TurnResult{}, err
	}
	lex, err := l.Store.RetrieveByLexical(ctx, userID, userMsg, 5)
	if err != nil {
		return TurnResult{}, err
	}
	items := dedupe(append(vecItems, lex...))

	prof, err := l.Store.GetProfile(ctx, userID)
	if err != nil {
		return TurnResult{}, err
	}

	utts, err := l.Compactor.LoadRecent(ctx, userID, 20)
	if err != nil {
		return TurnResult{}, err
	}
	decision, err := l.Compactor.MaybeCompact(ctx, userID, utts, time.Time{})
	if err != nil {
		return TurnResult{}, err
	}

	reply, err := l.Bedrock.Chat(ctx, systemPrompt()+contextBlock(items, prof, utts), turns(utts, userMsg))
	if err != nil {
		return TurnResult{}, err
	}

	_ = l.Store.AppendUtterance(ctx, userID, "user", userMsg, runID)
	_ = l.Store.AppendUtterance(ctx, userID, "assistant", reply, runID)

	replyEmb, _ := l.Bedrock.Embed(ctx, reply)
	_, _ = l.Store.InsertNote(ctx, userID, "note", reply, replyEmb, "chat", 0)

	state, _ := json.Marshal(map[string]any{
		"recall":   items,
		"reply":    reply,
		"decision": decision,
	})
	_ = l.Store.SaveCheckpoint(ctx, runID, 0, state, len(userMsg)/4+len(reply)/4)

	if l.Audit != nil {
		l.Audit.Write(ctx, userID, "turn", map[string]any{
			"run_id": runID, "recall": len(items), "compaction": decision,
		})
	}
	_ = l.Store.EndRun(ctx, runID, "done")

	return TurnResult{
		Reply:     reply,
		RunID:     runID,
		Tokens:    len(userMsg)/4 + len(reply)/4,
		Recall:    items,
		Compaction: decision,
	}, nil
}

func systemPrompt() string {
	return "You are StillMind, a non-clinical lifestyle meditation coach. Be calm, brief, and reflective. " +
		"Never provide medical advice. Do not diagnose. Recall prior user preferences when relevant. " +
		"Answer in 2-4 short sentences. Never echo or repeat the context, memory sections, or the user's message back at them."
}

func contextBlock(rec []memory.Retrieved, prof *memory.Profile, utts []compaction.Item) string {
	var sb strings.Builder
	if len(rec) > 0 {
		sb.WriteString("\n\nRecalled memory (semantic + lexical):\n")
		for _, r := range rec {
			sb.WriteString("- [")
			sb.WriteString(r.Kind)
			sb.WriteString("] ")
			sb.WriteString(r.Body)
			sb.WriteByte('\n')
		}
	}
	if prof != nil {
		sb.WriteString("\nProfile facts: ")
		b, _ := json.Marshal(prof.Facts)
		sb.Write(b)
		sb.WriteString("\nGoals: ")
		sb.WriteString(strings.Join(prof.Goals, ", "))
	}
	if len(utts) > 6 {
		sb.WriteString("\nOlder conversation (for context only):\n")
		for i := len(utts) - 1; i >= 6; i-- {
			sb.WriteString(utts[i].Role)
			sb.WriteString(": ")
			sb.WriteString(utts[i].Body)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func turns(utts []compaction.Item, userMsg string) []bedrock.ChatMessage {
	msgs := make([]bedrock.ChatMessage, 0, 8)
	if len(utts) > 0 {
		start := len(utts) - 6
		if start < 0 {
			start = 0
		}
		for i := start; i < len(utts); i++ {
			msgs = append(msgs, bedrock.ChatMessage{Role: utts[i].Role, Content: utts[i].Body})
		}
	}
	msgs = append(msgs, bedrock.ChatMessage{Role: "user", Content: userMsg})
	return msgs
}

func dedupe(in []memory.Retrieved) []memory.Retrieved {
	seen := map[string]bool{}
	out := make([]memory.Retrieved, 0, len(in))
	for _, r := range in {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out
}
