package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/stillmind/backend/internal/agent"
	"github.com/stillmind/backend/internal/audit"
	"github.com/stillmind/backend/internal/bedrock"
	"github.com/stillmind/backend/internal/compaction"
	"github.com/stillmind/backend/internal/config"
	"github.com/stillmind/backend/internal/memory"
	"github.com/stillmind/backend/internal/plan"
)

type Server struct {
	Cfg      config.Config
	Store    *memory.Store
	Bedrock  *bedrock.Client
	Loop     *agent.Loop
	Compact  *compaction.Worker
	Plan     *plan.Builder
	Sheets   *plan.MockSheets
	Audit    *audit.Logger
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/chat", s.chat)
	mux.HandleFunc("/plan", s.createPlan)
	mux.HandleFunc("/plan/csv", s.planCSV)
	mux.HandleFunc("/sheets/sync", s.sheetsSync)
	mux.HandleFunc("/memory/recall", s.recall)
	mux.HandleFunc("/memory/notes", s.notes)
	mux.HandleFunc("/memory/audit", s.auditList)
	mux.HandleFunc("/memory/explain", s.explain)
	return logging(cors(mux))
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "content-type,authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "stillmind"})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	uid := s.Cfg.UserID()
	res, err := s.Loop.Step(r.Context(), uid, body.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid := s.Cfg.UserID()
	lockKey := "plan:" + uid
	got, err := s.Store.AcquireLock(r.Context(), lockKey, uid, "planner", 30)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !got {
		http.Error(w, "another agent is creating the plan", http.StatusConflict)
		return
	}
	id, items, err := s.Plan.CreateWeek(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.Store.LogTool(r.Context(), uid, "", "plan.create", []byte(`{}`), []byte(`{"plan_id":"`+id+`"}`))
	writeJSON(w, http.StatusOK, map[string]any{"plan_id": id, "items": items})
}

func (s *Server) planCSV(w http.ResponseWriter, r *http.Request) {
	planID := r.URL.Query().Get("plan_id")
	if planID == "" {
		http.Error(w, "plan_id required", http.StatusBadRequest)
		return
	}
	items, err := s.Store.ListPlanItems(r.Context(), planID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=plan-"+planID+".csv")
	_, _ = w.Write([]byte(plan.ToCSV(items)))
}

func (s *Server) sheetsSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	planID := r.URL.Query().Get("plan_id")
	if planID == "" {
		http.Error(w, "plan_id required", http.StatusBadRequest)
		return
	}
	ref, err := s.Sheets.Sync(r.Context(), s.Cfg.UserID(), planID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ref": ref})
}

func (s *Server) recall(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "q required", http.StatusBadRequest)
		return
	}
	uid := s.Cfg.UserID()
	emb, err := s.Bedrock.Embed(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vec, err := s.Store.RetrieveByVector(r.Context(), uid, emb, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lex, err := s.Store.RetrieveByLexical(r.Context(), uid, q, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vector": vec, "lexical": lex})
}

func (s *Server) notes(w http.ResponseWriter, r *http.Request) {
	uid := s.Cfg.UserID()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.Store.Pool.Query(r.Context(), `
		SELECT id::text, kind, body, created_at::text FROM notes
		WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, uid, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type n struct {
		ID, Kind, Body, CreatedAt string
	}
	var out []n
	for rows.Next() {
		var x n
		if err := rows.Scan(&x.ID, &x.Kind, &x.Body, &x.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	uid := s.Cfg.UserID()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Store.Pool.Query(r.Context(), `
		SELECT id::text, type, payload::text, ts::text FROM audit_events
		WHERE user_id = $1 ORDER BY ts DESC LIMIT $2
	`, uid, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type a struct{ ID, Type, Payload, TS string }
	var out []a
	for rows.Next() {
		var x a
		if err := rows.Scan(&x.ID, &x.Type, &x.Payload, &x.TS); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) explain(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "q required", http.StatusBadRequest)
		return
	}
	uid := s.Cfg.UserID()
	emb, err := s.Bedrock.Embed(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plan, err := s.Store.ExplainVectorQuery(r.Context(), uid, emb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: 200}
		h.ServeHTTP(ww, r)
		_ = start
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) {
	s.status = c
	s.ResponseWriter.WriteHeader(c)
}

func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:    ":" + s.Cfg.Port,
		Handler: s.Routes(),
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}
