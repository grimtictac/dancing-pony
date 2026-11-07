// Package sentiment scores how positive or negative a piece of text is.
//
// Scorer is the seam: the built-in WordList scorer is a small, fast,
// deterministic baseline (the same idea as VADER, much simplified). A model
// such as an LLM can be added later behind the same interface.
package sentiment

import (
	"context"
	"math"
	"strings"
	"unicode"
)

// Sentiment is the result of scoring one text.
type Sentiment struct {
	Score float64 // -1 (very negative) to 1 (very positive)
	Label string  // "positive", "neutral" or "negative"
	Model string  // which scorer produced it, e.g. "wordlist-v1"
}

// Go note: an interface lists methods only. Any type with a matching Score
// method satisfies it implicitly; there is no "implements" keyword.

// Scorer scores text. Implementations must be safe for concurrent use.
type Scorer interface {
	Score(ctx context.Context, text string) (Sentiment, error)
}

// WordListModel identifies scores produced by WordList. It's stored with
// every score, so scores from different scorers are never mixed silently.
const WordListModel = "wordlist-v1"

// WordList scores text by summing the weights of known words, with simple
// handling of negation ("not good") and intensifiers ("very good").
type WordList struct{}

// Go note: assigning to the blank identifier is a compile-time check that
// WordList satisfies Scorer; the build fails if it ever stops doing so.
var _ Scorer = WordList{}

var lexicon = map[string]float64{
	"amazing": 3, "awesome": 3, "delicious": 3, "excellent": 3, "fantastic": 3,
	"great": 3, "love": 3, "loved": 3, "perfect": 3, "wonderful": 3,
	"enjoy": 2, "enjoyed": 2, "good": 2, "lovely": 2, "tasty": 2, "yummy": 2,
	"fresh": 1, "nice": 1, "fine": 1, "generous": 1,
	"bland": -1, "cold": -1, "slow": -1, "raw": -1, "meh": -1,
	"bad": -2, "burnt": -2, "overpriced": -2, "rude": -2, "soggy": -2, "stale": -2,
	"awful": -3, "disgusting": -3, "gross": -3, "hate": -3, "horrible": -3,
	"inedible": -3, "terrible": -3, "worst": -3,
}

var intensifiers = map[string]float64{
	"very": 1.5, "really": 1.5, "so": 1.3, "extremely": 2, "incredibly": 2,
	"slightly": 0.5, "somewhat": 0.7,
}

var negations = map[string]bool{
	"not": true, "no": true, "never": true, "hardly": true, "without": true,
}

// Score implements Scorer. It never fails; the error is for scorers that can.
func (WordList) Score(_ context.Context, text string) (Sentiment, error) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	})

	var sum float64
	for i, w := range words {
		weight, ok := lexicon[w]
		if !ok {
			continue
		}
		if i > 0 {
			if m, ok := intensifiers[words[i-1]]; ok {
				weight *= m
			}
		}
		// A negation within the three words before flips and damps the
		// weight: "not good" is mildly negative, not as bad as "bad".
		for j := max(0, i-3); j < i; j++ {
			if negations[words[j]] || strings.HasSuffix(words[j], "n't") {
				weight *= -0.75
				break
			}
		}
		sum += weight
	}

	// Squash the unbounded sum into (-1, 1), as VADER does.
	score := sum / math.Sqrt(sum*sum+15)
	return Sentiment{Score: score, Label: label(score), Model: WordListModel}, nil
}

func label(score float64) string {
	switch {
	case score >= 0.05:
		return "positive"
	case score <= -0.05:
		return "negative"
	default:
		return "neutral"
	}
}
