// Package modelfingerprint ranks numerical responses against frozen reference banks.
// Algorithm adapted from ModelTrace (MIT); attribution is in data/MODELTRACE-LICENSE.
package modelfingerprint

import (
	"embed"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

//go:embed data/*
var files embed.FS

type feature struct {
	Mean         []float64     `json:"feature_mean"`
	Scale        []float64     `json:"feature_scale"`
	Basis        [][]float64   `json:"nuisance_basis"`
	Centroids    [][]float64   `json:"centroids"`
	Environments [][][]float64 `json:"environment_centroids"`
	Weight       float64       `json:"weight"`
}
type bank struct {
	Robust struct {
		Models    []string `json:"model_order"`
		Hellinger feature  `json:"hellinger"`
		Ordered   feature  `json:"ordered_blocks"`
	} `json:"robust"`
}
type Candidate struct {
	Model string  `json:"model"`
	Score float64 `json:"score"`
}
type Challenge struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
	Count  int    `json:"expected_count"`
}
type Sample struct {
	ID         string      `json:"id"`
	Text       string      `json:"text"`
	Count      int         `json:"count"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Error      string      `json:"error,omitempty"`
}

var digits = regexp.MustCompile(`[0-9]+`)

func Challenges() ([]Challenge, error) {
	data, err := files.ReadFile("data/challenges.json")
	if err != nil {
		return nil, err
	}
	var result []Challenge
	err = common.Unmarshal(data, &result)
	return result, err
}
func Numbers(text string) []int {
	var longest, current []int
	previous := 0
	for _, location := range digits.FindAllStringIndex(text, -1) {
		if strings.IndexFunc(text[previous:location[0]], unicode.IsLetter) >= 0 {
			if len(current) > len(longest) {
				longest = current
			}
			current = nil
		}
		n, err := strconv.Atoi(text[location[0]:location[1]])
		if err == nil && n >= 1 && n <= 355 {
			current = append(current, n)
		}
		previous = location[1]
	}
	if len(current) > len(longest) {
		longest = current
	}
	return longest
}
func standardize(v []float64) []float64 {
	mean := 0.0
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	variance := 0.0
	for _, x := range v {
		variance += (x - mean) * (x - mean)
	}
	scale := math.Max(math.Sqrt(variance/float64(len(v))), 1e-12)
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = (x - mean) / scale
	}
	return out
}
func dot(a, b []float64) float64 {
	v := 0.0
	for i, x := range a {
		v += x * b[i]
	}
	return v
}
func normalize(v []float64) []float64 {
	out := append([]float64(nil), v...)
	norm := math.Max(math.Sqrt(dot(v, v)), 1e-12)
	for i := range out {
		out[i] /= norm
	}
	return out
}
func smooth(counts []float64) []float64 {
	sum := float64(len(counts)) * .5
	for _, x := range counts {
		sum += x
	}
	for i := range counts {
		counts[i] = math.Sqrt((counts[i] + .5) / sum)
	}
	return counts
}
func featureScores(values []float64, f feature, templates bool) []float64 {
	v := make([]float64, len(values))
	for i, x := range values {
		v[i] = (x - f.Mean[i]) / f.Scale[i]
	}
	projected := append([]float64(nil), v...)
	for _, axis := range f.Basis {
		weight := dot(v, axis)
		for i := range projected {
			projected[i] -= weight * axis[i]
		}
	}
	projected = normalize(projected)
	scores := make([]float64, len(f.Centroids))
	for i, center := range f.Centroids {
		scores[i] = dot(projected, center)
	}
	scores = standardize(scores)
	if !templates {
		return standardize(scores)
	}
	normalized := normalize(v)
	best := make([]float64, len(scores))
	for i := range best {
		best[i] = math.Inf(-1)
	}
	for _, environment := range f.Environments {
		for i, center := range environment {
			best[i] = math.Max(best[i], dot(normalized, center))
		}
	}
	best = standardize(best)
	for i := range scores {
		scores[i] = .5*scores[i] + .5*best[i]
	}
	return standardize(scores)
}
func Rank(text, family string, expected int) (Sample, error) {
	sample := Sample{Text: text}
	numbers := Numbers(text)
	sample.Count = len(numbers)
	if len(numbers) < 80 || len(numbers) < int(math.Ceil(float64(expected)*.55)) {
		sample.Error = "insufficient_numbers"
		return sample, nil
	}
	if family != "claude" && family != "gpt" {
		return sample, fmt.Errorf("unsupported reference family")
	}
	data, err := files.ReadFile("data/" + family + ".json")
	if err != nil {
		return sample, err
	}
	var b bank
	if err = common.Unmarshal(data, &b); err != nil {
		return sample, err
	}
	counts := make([]float64, 355)
	for _, n := range numbers {
		counts[n-1]++
	}
	marginal := featureScores(smooth(counts), b.Robust.Hellinger, false)
	ordered := make([]float64, 0, 74)
	offset := 0
	for block := 0; block < 4; block++ {
		size := len(numbers) / 4
		if block < len(numbers)%4 {
			size++
		}
		bins := make([]float64, 16)
		for _, n := range numbers[offset : offset+size] {
			bins[(n-1)*16/355]++
		}
		offset += size
		ordered = append(ordered, smooth(bins)...)
	}
	last := make([]float64, 10)
	for _, n := range numbers {
		last[n%10]++
	}
	ordered = append(ordered, smooth(last)...)
	sequence := featureScores(ordered, b.Robust.Ordered, true)
	weight := b.Robust.Ordered.Weight
	for i, model := range b.Robust.Models {
		if family == "gpt" && !strings.HasPrefix(model, "gpt-") {
			continue
		}
		sample.Candidates = append(sample.Candidates, Candidate{Model: model, Score: (1-weight)*marginal[i] + weight*sequence[i]})
	}
	sort.SliceStable(sample.Candidates, func(i, j int) bool { return sample.Candidates[i].Score > sample.Candidates[j].Score })
	return sample, nil
}
func Aggregate(samples []Sample) []Candidate {
	scores := map[string]float64{}
	count := 0
	for _, s := range samples {
		if len(s.Candidates) == 0 {
			continue
		}
		count++
		for _, c := range s.Candidates {
			scores[c.Model] += c.Score
		}
	}
	result := make([]Candidate, 0, len(scores))
	for model, score := range scores {
		result = append(result, Candidate{model, score / float64(count)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Model < result[j].Model
		}
		return result[i].Score > result[j].Score
	})
	return result
}
