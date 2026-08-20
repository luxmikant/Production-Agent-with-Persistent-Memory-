package plan

import (
	"context"
	"encoding/csv"
	"strings"
	"time"

	"github.com/stillmind/backend/internal/memory"
)

type Builder struct {
	Store *memory.Store
}

func (b *Builder) CreateWeek(ctx context.Context, userID string) (string, []memory.PlanItem, error) {
	id, err := b.Store.CreatePlan(ctx, userID, "7-day calm plan")
	if err != nil {
		return "", nil, err
	}
	items := defaultWeek()
	for _, it := range items {
		if err := b.Store.AddPlanItem(ctx, id, it); err != nil {
			return "", nil, err
		}
	}
	return id, items, nil
}

func defaultWeek() []memory.PlanItem {
	methods := []string{"box_breath", "body_scan", "walk", "loving_kindness", "box_breath", "body_scan", "walk"}
	slots := []string{"morning", "evening"}
	durations := []int{5, 10}
	notes := []string{"steady pace", "lower lights", "easy steps", "soft tone", "even rhythm", "wide awareness", "notice surroundings"}
	out := make([]memory.PlanItem, 0, 14)
	for d := 1; d <= 7; d++ {
		for s, slot := range slots {
			out = append(out, memory.PlanItem{
				DayOffset:   d,
				Slot:        slot,
				Method:      methods[(d+s)%len(methods)],
				DurationMin: durations[(d+s)%len(durations)],
				Notes:       notes[(d+s)%len(notes)],
			})
		}
	}
	return out
}

func ToCSV(items []memory.PlanItem) string {
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	_ = w.Write([]string{"day", "slot", "method", "duration_min", "notes"})
	for _, it := range items {
		_ = w.Write([]string{
			itoa(it.DayOffset),
			it.Slot,
			it.Method,
			itoa(it.DurationMin),
			it.Notes,
		})
	}
	w.Flush()
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type MockSheets struct {
	Store *memory.Store
}

func (s *MockSheets) Sync(ctx context.Context, userID, planID string) (string, error) {
	_, err := s.Store.ListPlanItems(ctx, planID)
	if err != nil {
		return "", err
	}
	ref := "sheet://" + userID + "/" + planID + "@" + time.Now().Format("20060102-150405")
	_ = s.Store.Audit(ctx, userID, "sheets.sync", []byte(`{"plan_id":"`+planID+`","ref":"`+ref+`"}`))
	return ref, nil
}
