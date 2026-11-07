package sentiment

import (
	"context"
	"strings"
	"testing"
)

func TestWordList(t *testing.T) {
	tests := []struct {
		text  string
		label string
	}{
		{"", "neutral"},
		{"It was a pie.", "neutral"},
		{"Delicious, best pie in Bree!", "positive"},
		{"Absolutely terrible. Cold and soggy.", "negative"},
		{"Not good at all", "negative"},
		{"wasn't bad", "positive"},
		{"I LOVED IT", "positive"},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			s, err := WordList{}.Score(context.Background(), tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if s.Label != tt.label || s.Model != WordListModel {
				t.Errorf("got %+v, want label %s", s, tt.label)
			}
			if s.Score < -1 || s.Score > 1 {
				t.Errorf("score %v outside [-1, 1]", s.Score)
			}
		})
	}
}

func TestIntensifiersStrengthen(t *testing.T) {
	good, _ := WordList{}.Score(context.Background(), "good")
	veryGood, _ := WordList{}.Score(context.Background(), "very good")
	if veryGood.Score <= good.Score {
		t.Errorf(`"very good" (%v) should score above "good" (%v)`, veryGood.Score, good.Score)
	}
}

// Go note: benchmarks are functions named BenchmarkXxx, run with
// go test -bench=. ; b.Loop runs the body as many times as needed to time it.
//
// It confirms D5: scoring inline in the request is cheap (microseconds), so
// it doesn't need a background queue.
func BenchmarkWordList(b *testing.B) {
	text := strings.Repeat("The mushroom pie was really delicious but the stew was cold. ", 20)
	for b.Loop() {
		_, _ = WordList{}.Score(context.Background(), text)
	}
}
