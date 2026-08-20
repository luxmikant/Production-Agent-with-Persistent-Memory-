package memory

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool     *pgxpool.Pool
	EmbedDim int
}

func Open(ctx context.Context, url string, embedDim int) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &Store{Pool: pool, EmbedDim: embedDim}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func (s *Store) Exec(ctx context.Context, q string, args ...any) error {
	_, err := s.Pool.Exec(ctx, q, args...)
	return err
}

func vecLit(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	sb.WriteByte(']')
	return sb.String()
}

func VectorParam(v []float32) string { return vecLit(v) }

type Retrieved struct {
	ID       string
	Body     string
	Kind     string
	Distance float32
}

func (s *Store) RetrieveByVector(ctx context.Context, userID string, q []float32, k int) ([]Retrieved, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, body, kind, (embedding <-> $1::vector) AS distance
		FROM notes
		WHERE user_id = $2 AND embedding IS NOT NULL
		ORDER BY embedding <-> $1::vector ASC
		LIMIT $3
	`, vecLit(q), userID, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Retrieved
	for rows.Next() {
		var r Retrieved
		if err := rows.Scan(&r.ID, &r.Body, &r.Kind, &r.Distance); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RetrieveByLexical(ctx context.Context, userID, query string, k int) ([]Retrieved, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, body, kind, 0.0::float8
		FROM notes
		WHERE user_id = $1 AND body ILIKE '%' || $2 || '%'
		ORDER BY created_at DESC
		LIMIT $3
	`, userID, q, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Retrieved
	for rows.Next() {
		var r Retrieved
		if err := rows.Scan(&r.ID, &r.Body, &r.Kind, &r.Distance); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) InsertNote(ctx context.Context, userID, kind, body string, emb []float32, source string, ttlHours int) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO notes (user_id, kind, body, embedding, source, ttl_until)
		VALUES ($1, $2, $3, $4::vector, $5,
		        CASE WHEN $6::int > 0 THEN now() + ($6::int || ' hours')::interval ELSE NULL END)
		RETURNING id::text
	`, userID, kind, body, vecLit(emb), source, ttlHours).Scan(&id)
	return id, err
}

func (s *Store) AppendUtterance(ctx context.Context, userID, role, body string, runID string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO raw_utterances (user_id, role, body, run_id)
		VALUES ($1, $2, $3, NULLIF($4::string, '')::uuid)
	`, userID, role, body, runID)
	return err
}

func (s *Store) StartRun(ctx context.Context, userID string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO agent_runs (user_id, status) VALUES ($1, 'active') RETURNING id::text
	`, userID).Scan(&id)
	return id, err
}

func (s *Store) EndRun(ctx context.Context, runID, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE agent_runs SET status=$2, ended_at=now() WHERE id=$1`, runID, status)
	return err
}

type Checkpoint struct {
	Step  int
	State []byte
}

func (s *Store) SaveCheckpoint(ctx context.Context, runID string, step int, state []byte, tokens int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO agent_checkpoints (run_id, step, state, tokens_used)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (run_id, step) DO UPDATE SET state = EXCLUDED.state, tokens_used = EXCLUDED.tokens_used, ts = now()
	`, runID, step, state, tokens)
	return err
}

func (s *Store) LatestCheckpoint(ctx context.Context, runID string) (Checkpoint, error) {
	var c Checkpoint
	err := s.Pool.QueryRow(ctx, `
		SELECT step, state FROM agent_checkpoints WHERE run_id = $1 ORDER BY step DESC LIMIT 1
	`, runID).Scan(&c.Step, &c.State)
	return c, err
}

func (s *Store) LogTool(ctx context.Context, userID, runID, tool string, args, result []byte) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO tool_history (user_id, run_id, tool, args, result)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)
	`, userID, runID, tool, args, result)
	return err
}

func (s *Store) Audit(ctx context.Context, userID, typ string, payload []byte) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO audit_events (user_id, type, payload) VALUES ($1, $2, $3)
	`, userID, typ, payload)
	return err
}

func (s *Store) CreatePlan(ctx context.Context, userID, title string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO plans (user_id, title) VALUES ($1, $2) RETURNING id::text
	`, userID, title).Scan(&id)
	return id, err
}

type PlanItem struct {
	DayOffset   int
	Slot        string
	Method      string
	DurationMin int
	Notes       string
}

func (s *Store) AddPlanItem(ctx context.Context, planID string, it PlanItem) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO plan_items (plan_id, day_offset, slot, method, duration_min, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (plan_id, day_offset, slot) DO UPDATE SET method = EXCLUDED.method, duration_min = EXCLUDED.duration_min, notes = EXCLUDED.notes
	`, planID, it.DayOffset, it.Slot, it.Method, it.DurationMin, it.Notes)
	return err
}

func (s *Store) ListPlanItems(ctx context.Context, planID string) ([]PlanItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT day_offset, slot, method, duration_min, COALESCE(notes,'')
		FROM plan_items WHERE plan_id = $1 ORDER BY day_offset, slot
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		var i PlanItem
		if err := rows.Scan(&i.DayOffset, &i.Slot, &i.Method, &i.DurationMin, &i.Notes); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

type Profile struct {
	UserID string
	Facts  map[string]any
	Goals  []string
	Prefs  map[string]any
}

func (s *Store) GetProfile(ctx context.Context, userID string) (*Profile, error) {
	var p Profile
	var facts, prefs []byte
	var goals []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT user_id, facts::text, goals::text, prefs::text FROM profiles WHERE user_id = $1
	`, userID).Scan(&p.UserID, &facts, &goals, &prefs)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &Profile{UserID: userID, Facts: map[string]any{}, Goals: []string{}, Prefs: map[string]any{}}, nil
		}
		return nil, err
	}
	parseJSON(facts, &p.Facts)
	parseJSON(goals, &p.Goals)
	parseJSON(prefs, &p.Prefs)
	return &p, nil
}

func (s *Store) UpdateProfileFacts(ctx context.Context, userID string, patch map[string]any) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE profiles SET facts = facts || $2::jsonb, updated_at = now() WHERE user_id = $1
	`, userID, mustJSON(patch))
	return err
}

func (s *Store) RecordCompact(ctx context.Context, userID, kind, summary string, emb []float32, inTok, outTok int, ratio, score float64, rawIDs []string, attempt int, status string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO compactions (user_id, kind, input_tokens, output_tokens, compression_ratio,
		                          validation_score, summary_text, summary_embedding, raw_ids, attempt, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::vector, $9, $10, $11)
	`, userID, kind, inTok, outTok, ratio, score, summary, vecLit(emb), rawIDs, attempt, status)
	return err
}

func (s *Store) RecentUtterances(ctx context.Context, userID string, limit int) ([]struct {
	ID   string
	Role string
	Body string
	TS   string
}, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, role, body, ts::text FROM raw_utterances
		WHERE user_id = $1 ORDER BY ts DESC LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID, Role, Body, TS string
	}
	for rows.Next() {
		var r struct{ ID, Role, Body, TS string }
		if err := rows.Scan(&r.ID, &r.Role, &r.Body, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ExpireRawUtterances(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE raw_utterances SET ttl_until = now()
		WHERE id::text = ANY($1)
	`, ids)
	return err
}

func (s *Store) AcquireLock(ctx context.Context, resource, userID, owner string, ttlSeconds int) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO resource_locks (resource, user_id, owner, expires_at)
		VALUES ($1, $2, $3, now() + ($4::int || ' seconds')::interval)
		ON CONFLICT (resource, user_id) DO UPDATE
		  SET owner = EXCLUDED.owner, expires_at = EXCLUDED.expires_at
		  WHERE resource_locks.expires_at < now()
	`, resource, userID, owner, ttlSeconds)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) PrefetchGet(ctx context.Context, userID, intentKey string) (string, []byte, []float32, error) {
	var body string
	var ctxBytes []byte
	var emb []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT context::text, context, embedding::text FROM prefetch_cache
		WHERE user_id = $1 AND intent_key = $2 AND expires_at > now()
	`, userID, intentKey).Scan(&body, &ctxBytes, &emb)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil, nil, nil
		}
		return "", nil, nil, err
	}
	vec := parseVecText(emb)
	return body, ctxBytes, vec, nil
}

func (s *Store) PrefetchPut(ctx context.Context, userID, intentKey string, payload []byte, emb []float32, ttlSeconds int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO prefetch_cache (user_id, intent_key, context, embedding, expires_at)
		VALUES ($1, $2, $3, $4::vector, now() + ($5::int || ' seconds')::interval)
		ON CONFLICT (user_id, intent_key) DO UPDATE
		  SET context = EXCLUDED.context, embedding = EXCLUDED.embedding, expires_at = EXCLUDED.expires_at
	`, userID, intentKey, payload, vecLit(emb), ttlSeconds)
	return err
}

func (s *Store) ExplainVectorQuery(ctx context.Context, userID string, q []float32) (string, error) {
	rows, err := s.Pool.Query(ctx, `EXPLAIN SELECT id FROM notes WHERE user_id = $1 ORDER BY embedding <-> $2::vector LIMIT 3`, userID, vecLit(q))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func parseVecText(b []byte) []float32 {
	s := strings.Trim(string(b), "[]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			continue
		}
		out = append(out, float32(f))
	}
	return out
}

func mustJSON(v any) string {
	b, err := jsonMarshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func parseJSON(b []byte, out any) {
	if len(b) == 0 {
		return
	}
	_ = jsonUnmarshal(b, out)
}

func MapToStringKeys(m map[string]any) string { return mustJSON(m) }
func StringListToJSON(s []string) string     { return mustJSON(s) }
func StringToJSON(s string) string           { return fmt.Sprintf("%q", s) }
