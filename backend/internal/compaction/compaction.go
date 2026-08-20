package compaction

import (
	"context"
	"strings"
	"time"

	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/memory"
)

type Worker struct {
	Store     *memory.Store
	Bedrock   *bedrock.Client
	Budget    int
	Pressure  float64
	MinS      int
	Threshold float64
}

type Decision struct {
	Fired       bool
	Reason      string
	InputTokens int
	OutputTokens int
	Score       float64
	Ratio       float64
}

func (w *Worker) PressureOf(items []Item) float64 {
	toks := 0
	for _, it := range items {
		toks += len(it.Body) / 4
	}
	if w.Budget == 0 {
		return 0
	}
	return float64(toks) / float64(w.Budget)
}

type Item struct {
	ID   string
	Role string
	Body string
}

func (w *Worker) LoadRecent(ctx context.Context, userID string, n int) ([]Item, error) {
	rows, err := w.Store.RecentUtterances(ctx, userID, n)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, Item{ID: r.ID, Role: r.Role, Body: r.Body})
	}
	return out, nil
}

func (w *Worker) MaybeCompact(ctx context.Context, userID string, items []Item, lastRun time.Time) (Decision, error) {
	now := time.Now()
	if now.Sub(lastRun) < time.Duration(w.MinS)*time.Second {
		return Decision{Reason: "min-interval"}, nil
	}
	p := w.PressureOf(items)
	if p < w.Pressure {
		return Decision{Reason: "below-pressure", InputTokens: tokensOf(items)}, nil
	}

	inTok := tokensOf(items)
	for attempt := 1; attempt <= 3; attempt++ {
		summary, err := w.Bedrock.Chat(ctx, compactPrompt(items, attempt), []bedrock.ChatMessage{
			{Role: "user", Content: "Produce the summary."},
		})
		if err != nil {
			return Decision{}, err
		}
		emb, err := w.Bedrock.Embed(ctx, summary)
		if err != nil {
			return Decision{}, err
		}
		score, err := w.validate(ctx, items, summary)
		if err != nil {
			return Decision{}, err
		}
		outTok := len(summary) / 4
		ratio := float64(outTok) / float64(inTok+1)
		status := "ok"
		if score < w.Threshold {
			status = "retried"
		}
		ids := make([]string, len(items))
		for i, it := range items {
			ids[i] = it.ID
		}
		_ = w.Store.RecordCompact(ctx, userID, "session", summary, emb, inTok, outTok, ratio, score, ids, attempt, status)
		if score >= w.Threshold {
			_ = w.Store.ExpireRawUtterances(ctx, ids)
			return Decision{Fired: true, Reason: "ok", InputTokens: inTok, OutputTokens: outTok, Score: score, Ratio: ratio}, nil
		}
	}
	return Decision{Reason: "below-threshold-after-retries", InputTokens: inTok}, nil
}

func (w *Worker) validate(ctx context.Context, items []Item, summary string) (float64, error) {
	probe := "Reply with a JSON object {\"score\": <0-1 float>, \"missing\": [<short strings>]}. Score how many of the user's stated facts/preferences from the original turns are recoverable from the summary. Be strict."
	resp, err := w.Bedrock.Chat(ctx, probe, []bedrock.ChatMessage{
		{Role: "user", Content: "Original turns:\n" + joinItems(items) + "\n\nSummary:\n" + summary},
	})
	if err != nil {
		return 0, err
	}
	score := parseScore(resp)
	return score, nil
}

func tokensOf(items []Item) int {
	t := 0
	for _, it := range items {
		t += len(it.Body) / 4
	}
	return t
}

func joinItems(items []Item) string {
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString("[")
		sb.WriteString(it.Role)
		sb.WriteString("] ")
		sb.WriteString(it.Body)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func compactPrompt(items []Item, attempt int) string {
	aggression := "moderate"
	switch attempt {
	case 1:
		aggression = "moderate"
	case 2:
		aggression = "lighter (preserve more detail)"
	default:
		aggression = "minimal (preserve verbatim where possible)"
	}
	return "You are compressing a sequence of meditation-coaching turns. Produce a concise summary that preserves any user preferences, goals, and constraints. Aggression: " + aggression + "."
}

func parseScore(s string) float64 {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\"score\""); i >= 0 {
		rest := s[i+7:]
		if j := strings.IndexAny(rest, "0123456789"); j >= 0 {
			rest = rest[j:]
			end := 0
			for end < len(rest) && (rest[end] >= '0' && rest[end] <= '9' || rest[end] == '.') {
				end++
			}
			if end > 0 {
				v := 0.0
				_, err := fmtSscan(rest[:end], &v)
				if err == nil {
					return v
				}
			}
		}
	}
	return 0.5
}

func fmtSscan(s string, v *float64) (int, error) {
	var f float64
	var n int
	for ; n < len(s); n++ {
		c := s[n]
		if (c >= '0' && c <= '9') || c == '.' || (n == 0 && c == '-') {
			f = f*10 + float64(c-'0')
			if c == '.' {
				div := 10.0
				n++
				for ; n < len(s); n++ {
					c := s[n]
					if c < '0' || c > '9' {
						break
					}
					f += float64(c-'0') / div
					div *= 10
				}
				*v = f
				return n, nil
			}
		} else {
			break
		}
	}
	*v = f
	return n, nil
}
