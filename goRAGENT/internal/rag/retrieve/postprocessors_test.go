package retrieve

import (
	"context"
	"testing"
)

func TestDedup(t *testing.T) {
	d := &DedupPostProcessor{}
	chunks := []RetrievedChunk{
		{ID: "a", Text: "first", Score: 0.9},
		{ID: "a", Text: "first", Score: 0.8}, // duplicate
		{ID: "b", Text: "second", Score: 0.7},
		{ID: "b", Text: "second", Score: 0.6}, // duplicate
		{ID: "c", Text: "third", Score: 0.5},
	}
	result, err := d.Process(context.Background(), chunks, nil)
	if err != nil { t.Fatal(err) }
	if len(result) != 3 { t.Errorf("expected 3 unique, got %d", len(result)) }
}

func TestFusion_RRF(t *testing.T) {
	f := &FusionPostProcessor{RRFK: 60, RerankCandidateLimit: 0}
	chunks := []RetrievedChunk{
		{ID: "a", Text: "top", Score: 0.95},
		{ID: "b", Text: "mid", Score: 0.80},
		{ID: "c", Text: "low", Score: 0.60},
	}
	result, err := f.Process(context.Background(), chunks, nil)
	if err != nil { t.Fatal(err) }
	if len(result) != 3 { t.Errorf("expected 3, got %d", len(result)) }
	// RRF should preserve order but re-score
	if result[0].ID != "a" { t.Errorf("expected highest RRF first, got %s", result[0].ID) }
}

func TestFusion_CandidateLimit(t *testing.T) {
	f := &FusionPostProcessor{RRFK: 60, RerankCandidateLimit: 2}
	chunks := make([]RetrievedChunk, 10)
	for i := range chunks {
		chunks[i] = RetrievedChunk{ID: string(rune('a' + i)), Text: "chunk", Score: float64(10 - i)}
	}
	result, err := f.Process(context.Background(), chunks, nil)
	if err != nil { t.Fatal(err) }
	if len(result) != 2 { t.Errorf("expected 2 after limit, got %d", len(result)) }
}
