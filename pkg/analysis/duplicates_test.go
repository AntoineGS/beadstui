package analysis

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// ============================================================================
// extractKeywords Tests
// ============================================================================

func TestExtractKeywords_BasicExtraction(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		description string
		wantMinLen  int    // minimum keywords expected
		wantContain string // must contain this keyword
	}{
		{
			name:        "simple title only",
			title:       "Fix login button",
			description: "",
			wantMinLen:  2,
			wantContain: "login",
		},
		{
			name:        "title and description",
			title:       "Authentication fails",
			description: "Users cannot login when using OAuth provider",
			wantMinLen:  4,
			wantContain: "authentication",
		},
		{
			name:        "filters stop words",
			title:       "The user should be able to login",
			description: "This is a very important feature that will help users",
			wantMinLen:  2,
			wantContain: "user",
		},
		{
			name:        "handles special characters",
			title:       "Fix bug #123: crash on startup",
			description: "Error: null pointer @ line 42",
			wantMinLen:  3,
			wantContain: "crash",
		},
		{
			name:        "case insensitive",
			title:       "UPPERCASE Title Here",
			description: "MixedCase Description",
			wantMinLen:  2,
			wantContain: "uppercase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keywords := extractKeywords(tt.title, tt.description)
			if len(keywords) < tt.wantMinLen {
				t.Errorf("extractKeywords() returned %d keywords, want at least %d", len(keywords), tt.wantMinLen)
			}
			found := false
			for _, kw := range keywords {
				if kw == tt.wantContain {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractKeywords() missing expected keyword %q, got %v", tt.wantContain, keywords)
			}
		})
	}
}

func TestDetectDuplicates_FullSortOracleParity(t *testing.T) {
	tests := []struct {
		name   string
		issues []model.Issue
		config DuplicateConfig
	}{
		{name: "dense", issues: duplicateBenchmarkIssues(48, true, 17), config: duplicateOracleConfig(20)},
		{name: "sparse", issues: duplicateBenchmarkIssues(80, false, 29), config: duplicateOracleConfig(20)},
		{name: "fixed random", issues: duplicateRandomIssues(64, 39), config: duplicateOracleConfig(20)},
		{name: "ties and above pair count", issues: duplicateBenchmarkIssues(5, true, 3), config: duplicateOracleConfig(100)},
		{name: "canonical tie ordering", issues: []model.Issue{
			{ID: "z", Title: "alpha birch cedar", Status: model.StatusOpen},
			{ID: "b", Title: "alpha birch cedar", Status: model.StatusOpen},
			{ID: "a", Title: "alpha birch cedar", Status: model.StatusOpen},
		}, config: duplicateOracleConfig(20)},
		{name: "threshold filtering", issues: []model.Issue{
			{ID: "alpha", Title: "alpha birch cedar amber violet", Status: model.StatusOpen},
			{ID: "beta", Title: "alpha birch cedar amber violet", Status: model.StatusOpen},
			{ID: "gamma", Title: "alpha birch cedar amber violet copper silver", Status: model.StatusOpen},
		}, config: DuplicateConfig{JaccardThreshold: 0.8, MinKeywords: 2, IgnoreClosedVsOpen: true, MaxSuggestions: 20}},
		{name: "repeated IDs preserve source explanations", issues: []model.Issue{
			{ID: "same", Title: "alpha birch cedar amber violet", Status: model.StatusOpen},
			{ID: "middle", Title: "alpha birch cedar amber violet", Status: model.StatusOpen},
			{ID: "same", Title: "unique copper silver quartz", Status: model.StatusOpen},
		}, config: duplicateOracleConfig(20)},
		{name: "status filters", issues: []model.Issue{
			{ID: "a", Title: "alpha birch cedar", Status: model.StatusOpen},
			{ID: "b", Title: "alpha birch cedar", Status: model.StatusClosed},
			{ID: "c", Title: "alpha birch cedar", Status: model.StatusTombstone},
			{ID: "d", Title: "alpha birch cedar", Status: model.StatusInProgress},
		}, config: duplicateOracleConfig(20)},
	}
	for _, tt := range tests {
		limits := []int{0, 1, 20, tt.config.MaxSuggestions}
		if tt.name == "repeated IDs preserve source explanations" {
			limits = []int{1}
		}
		for _, limit := range limits {
			t.Run(fmt.Sprintf("%s/k=%d", tt.name, limit), func(t *testing.T) {
				config := tt.config
				config.MaxSuggestions = limit
				got := DetectDuplicates(tt.issues, config)
				want := detectDuplicatesFullSortOracle(tt.issues, config)
				if !reflect.DeepEqual(withoutSuggestionTimes(got), withoutSuggestionTimes(want)) {
					t.Fatalf("suggestions differ:\ngot  %#v\nwant %#v", got, want)
				}
				if tt.name == "repeated IDs preserve source explanations" && len(got) > 0 && !strings.Contains(got[0].Reason, "alpha, amber, birch, cedar, violet") {
					t.Fatalf("repeated-ID pair used the wrong source records for explanation: %q", got[0].Reason)
				}
			})
		}
	}
	got := DetectDuplicates(tests[4].issues, tests[4].config)
	wantPairs := [][2]string{{"a", "b"}, {"a", "z"}, {"b", "z"}}
	if len(got) != len(wantPairs) {
		t.Fatalf("canonical tie fixture returned %d suggestions, want %d", len(got), len(wantPairs))
	}
	for i, pair := range wantPairs {
		if got[i].TargetBead != pair[0] || got[i].RelatedBead != pair[1] {
			t.Errorf("tie suggestion %d is (%q, %q), want (%q, %q)", i, got[i].TargetBead, got[i].RelatedBead, pair[0], pair[1])
		}
	}
}

func TestDetectDuplicates_NonpositiveKReturnsEmpty(t *testing.T) {
	issues := duplicateBenchmarkIssues(5, true, 3)
	for _, k := range []int{0, -1} {
		if got := DetectDuplicates(issues, duplicateOracleConfig(k)); len(got) != 0 {
			t.Errorf("DetectDuplicates(K=%d) returned %d suggestions, want none", k, len(got))
		}
	}
}

func duplicateOracleConfig(k int) DuplicateConfig {
	return DuplicateConfig{JaccardThreshold: 0.35, MinKeywords: 2, IgnoreClosedVsOpen: true, MaxSuggestions: k}
}

func withoutSuggestionTimes(suggestions []Suggestion) []Suggestion {
	result := append([]Suggestion(nil), suggestions...)
	for i := range result {
		result[i].GeneratedAt = time.Time{}
	}
	return result
}

// detectDuplicatesFullSortOracle preserves the pre-optimization algorithm:
// accumulate every qualifying pair with its source records, full-sort, truncate,
// and only then construct suggestions. It deliberately does not use a heap or
// the production candidate comparator.
func detectDuplicatesFullSortOracle(issues []model.Issue, config DuplicateConfig) []Suggestion {
	if len(issues) < 2 || config.MaxSuggestions <= 0 {
		return nil
	}

	keywords := make([][]string, len(issues))
	index := make(map[string][]int)
	seenIDs := make(map[string]struct{}, len(issues))
	repeatedIDs := false
	for i := range issues {
		if _, exists := seenIDs[issues[i].ID]; exists {
			repeatedIDs = true
		}
		seenIDs[issues[i].ID] = struct{}{}
	}
	for i := range issues {
		keywords[i] = extractKeywords(issues[i].Title, issues[i].Description)
		if len(keywords[i]) >= config.MinKeywords {
			for _, keyword := range keywords[i] {
				index[keyword] = append(index[keyword], i)
			}
		}
	}

	type oraclePair struct {
		pair        DuplicatePair
		left, right int
	}
	pairs := make([]oraclePair, 0)
	for left := range issues {
		if len(keywords[left]) < config.MinKeywords {
			continue
		}
		overlaps := make(map[int]int)
		for _, keyword := range keywords[left] {
			for _, right := range index[keyword] {
				if right > left {
					overlaps[right]++
				}
			}
		}
		for right, overlap := range overlaps {
			if len(keywords[right]) < config.MinKeywords {
				continue
			}
			union := len(keywords[left]) + len(keywords[right]) - overlap
			score := float64(overlap) / float64(union)
			if score < config.JaccardThreshold {
				continue
			}
			first, second := &issues[left], &issues[right]
			if first.Status == model.StatusTombstone || second.Status == model.StatusTombstone {
				continue
			}
			firstClosed := first.Status == model.StatusClosed || first.Status == model.StatusTombstone
			secondClosed := second.Status == model.StatusClosed || second.Status == model.StatusTombstone
			if config.IgnoreClosedVsOpen && firstClosed != secondClosed {
				continue
			}
			commonSet := make(map[string]struct{}, len(keywords[left]))
			for _, keyword := range keywords[left] {
				commonSet[keyword] = struct{}{}
			}
			common := make([]string, 0, len(commonSet))
			for _, keyword := range keywords[right] {
				if _, ok := commonSet[keyword]; ok {
					common = append(common, keyword)
				}
			}
			sort.Strings(common)
			firstID, secondID := first.ID, second.ID
			if !repeatedIDs && secondID < firstID {
				firstID, secondID = secondID, firstID
			}
			pairs = append(pairs, oraclePair{
				pair: DuplicatePair{Issue1: firstID, Issue2: secondID, Similarity: score, Method: "jaccard", Keywords: common},
				left: left, right: right,
			})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].pair.Similarity != pairs[j].pair.Similarity {
			return pairs[i].pair.Similarity > pairs[j].pair.Similarity
		}
		if pairs[i].pair.Issue1 != pairs[j].pair.Issue1 {
			return pairs[i].pair.Issue1 < pairs[j].pair.Issue1
		}
		return pairs[i].pair.Issue2 < pairs[j].pair.Issue2
	})
	if len(pairs) > config.MaxSuggestions {
		pairs = pairs[:config.MaxSuggestions]
	}
	issueByID := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		issueByID[issues[i].ID] = &issues[i]
	}
	suggestions := make([]Suggestion, 0, len(pairs))
	for _, candidate := range pairs {
		pair := candidate.pair
		first, second := issueByID[pair.Issue1], issueByID[pair.Issue2]
		suggestion := NewSuggestion(
			SuggestionPotentialDuplicate,
			pair.Issue1,
			fmt.Sprintf("Potential duplicate of %s", pair.Issue2),
			fmt.Sprintf("%.0f%% keyword similarity; common: %s", pair.Similarity*100, strings.Join(truncateStringSlice(pair.Keywords, 5), ", ")),
			pair.Similarity,
		).WithRelatedBead(pair.Issue2).WithMetadata("method", pair.Method)
		if first.Status != model.StatusClosed && first.Status != model.StatusTombstone &&
			second.Status != model.StatusClosed && second.Status != model.StatusTombstone {
			suggestion = suggestion.WithAction(fmt.Sprintf("bd dep add %s %s --type=related", pair.Issue1, pair.Issue2))
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions
}

func duplicateBenchmarkIssues(count int, dense bool, seed int64) []model.Issue {
	rng := rand.New(rand.NewSource(seed))
	issues := make([]model.Issue, count)
	for i := range issues {
		words := []string{fmt.Sprintf("subject%d", i)}
		if dense {
			words = append(words, "shared birch cedar amber violet")
		} else if i%8 == 0 {
			words = append(words, "shared sparsecluster")
		}
		for word := 0; word < 6; word++ {
			words = append(words, fmt.Sprintf("token%d", rng.Intn(25)))
		}
		issues[i] = model.Issue{ID: fmt.Sprintf("fixture-%04d", i), Title: strings.Join(words, " "), Status: model.StatusOpen}
	}
	return issues
}

func duplicateRandomIssues(count int, seed int64) []model.Issue {
	rng := rand.New(rand.NewSource(seed))
	vocabulary := []string{"amber", "birch", "cedar", "delta", "ember", "fable", "glacier", "harbor", "indigo", "juniper", "kernel", "lilac"}
	statuses := []model.Status{model.StatusOpen, model.StatusInProgress, model.StatusClosed, model.StatusTombstone}
	issues := make([]model.Issue, count)
	for i := range issues {
		words := make([]string, 5+rng.Intn(5))
		for j := range words {
			words[j] = vocabulary[rng.Intn(len(vocabulary))]
		}
		issues[i] = model.Issue{ID: fmt.Sprintf("random-%04d", i), Title: strings.Join(words, " "), Status: statuses[rng.Intn(len(statuses))]}
	}
	return issues
}

func BenchmarkDetectDuplicatesSelection(b *testing.B) {
	for _, fixture := range []struct {
		name   string
		issues []model.Issue
	}{
		{name: "dense500", issues: duplicateBenchmarkIssues(500, true, 20261012)},
		{name: "sparse500", issues: duplicateBenchmarkIssues(500, false, 20261013)},
	} {
		b.Run(fixture.name+"/full-sort", func(b *testing.B) {
			benchmarkDuplicateSelection(b, fixture.issues, detectDuplicatesFullSortOracle)
		})
		b.Run(fixture.name+"/bounded", func(b *testing.B) {
			benchmarkDuplicateSelection(b, fixture.issues, DetectDuplicates)
		})
	}
}

func benchmarkDuplicateSelection(b *testing.B, issues []model.Issue, detect func([]model.Issue, DuplicateConfig) []Suggestion) {
	config := duplicateOracleConfig(20)
	want := withoutSuggestionTimes(detectDuplicatesFullSortOracle(issues, config))
	got := withoutSuggestionTimes(detect(issues, config))
	if !reflect.DeepEqual(got, want) {
		b.Fatalf("benchmark output differs from full-sort oracle: got %d suggestions, want %d", len(got), len(want))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detect(issues, config)
	}
}

func TestExtractKeywords_FiltersShortWords(t *testing.T) {
	keywords := extractKeywords("A to do it is", "")
	for _, kw := range keywords {
		if len(kw) < 3 {
			t.Errorf("extractKeywords() included short word %q (len %d)", kw, len(kw))
		}
	}
}

func TestExtractKeywords_RemovesDuplicates(t *testing.T) {
	keywords := extractKeywords("login login login", "login authentication login")
	count := 0
	for _, kw := range keywords {
		if kw == "login" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("extractKeywords() has %d occurrences of 'login', want 1", count)
	}
}

func TestExtractKeywords_EmptyInput(t *testing.T) {
	keywords := extractKeywords("", "")
	if len(keywords) != 0 {
		t.Errorf("extractKeywords() with empty input returned %d keywords, want 0", len(keywords))
	}
}

func TestExtractKeywords_StopWordsFiltered(t *testing.T) {
	keywords := extractKeywords("the and for with this that", "from are was were been have")
	if len(keywords) > 0 {
		t.Errorf("extractKeywords() should filter stop words, got %v", keywords)
	}
}

// ============================================================================
// DetectDuplicates Tests - Exact Duplicates
// ============================================================================

func TestDetectDuplicates_IdenticalTitles(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "B", Title: "Implement user authentication system", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) == 0 {
		t.Fatal("DetectDuplicates() found no duplicates for identical titles")
	}

	sug := suggestions[0]
	if sug.Type != SuggestionPotentialDuplicate {
		t.Errorf("suggestion type = %v, want SuggestionPotentialDuplicate", sug.Type)
	}
	if sug.Confidence < 0.9 {
		t.Errorf("confidence = %v, want >= 0.9 for identical titles", sug.Confidence)
	}
}

func TestDetectDuplicates_IdenticalContent(t *testing.T) {
	issues := []model.Issue{
		{
			ID:          "A",
			Title:       "Fix login bug",
			Description: "Users cannot login when password contains special characters",
			Status:      model.StatusOpen,
		},
		{
			ID:          "B",
			Title:       "Login bug fix",
			Description: "Users cannot login when password contains special characters",
			Status:      model.StatusOpen,
		},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) == 0 {
		t.Fatal("DetectDuplicates() found no duplicates for identical descriptions")
	}
}

// ============================================================================
// DetectDuplicates Tests - Near Duplicates
// ============================================================================

func TestDetectDuplicates_SimilarTitles(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement authentication for API endpoints", Status: model.StatusOpen},
		{ID: "B", Title: "API endpoints need authentication implementation", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	config.JaccardThreshold = 0.5
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) == 0 {
		t.Fatal("DetectDuplicates() found no duplicates for similar titles")
	}
}

func TestDetectDuplicates_DifferentCase(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement User Authentication System", Status: model.StatusOpen},
		{ID: "B", Title: "implement user authentication system", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) == 0 {
		t.Fatal("DetectDuplicates() should be case-insensitive")
	}
}

// ============================================================================
// DetectDuplicates Tests - False Positive Prevention
// ============================================================================

func TestDetectDuplicates_ShortGenericTitles(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Fix bug", Status: model.StatusOpen},
		{ID: "B", Title: "Fix bug", Status: model.StatusOpen},
		{ID: "C", Title: "Update docs", Status: model.StatusOpen},
		{ID: "D", Title: "Update docs", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	config.MinKeywords = 3
	suggestions := DetectDuplicates(issues, config)

	for _, sug := range suggestions {
		if sug.TargetBead == "A" || sug.TargetBead == "C" {
			t.Logf("Note: Found potential false positive match for generic title")
		}
	}
}

func TestDetectDuplicates_CompletelyDifferent(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement OAuth2 authentication flow", Status: model.StatusOpen},
		{ID: "B", Title: "Database migration for user table", Status: model.StatusOpen},
		{ID: "C", Title: "Update CSS styling for mobile layout", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) > 0 {
		t.Errorf("DetectDuplicates() found %d duplicates for unrelated issues, want 0", len(suggestions))
	}
}

func TestDetectDuplicates_IgnoreClosedVsOpen(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "B", Title: "Implement user authentication system", Status: model.StatusClosed},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) > 0 {
		t.Error("DetectDuplicates() should ignore open vs closed pairs by default")
	}

	config.IgnoreClosedVsOpen = false
	suggestions = DetectDuplicates(issues, config)

	if len(suggestions) == 0 {
		t.Error("DetectDuplicates() should find duplicates when IgnoreClosedVsOpen=false")
	}
}

func TestDetectDuplicates_SkipsTombstone(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "B", Title: "Implement user authentication system", Status: model.StatusTombstone},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) > 0 {
		t.Error("DetectDuplicates() should skip tombstone issues")
	}
}

// ============================================================================
// DetectDuplicates Tests - Configuration
// ============================================================================

func TestDetectDuplicates_JaccardThreshold(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement authentication for users", Status: model.StatusOpen},
		{ID: "B", Title: "Implement authorization for admins", Status: model.StatusOpen},
	}

	highConfig := DefaultDuplicateConfig()
	highConfig.JaccardThreshold = 0.9
	highSuggestions := DetectDuplicates(issues, highConfig)

	lowConfig := DefaultDuplicateConfig()
	lowConfig.JaccardThreshold = 0.3
	lowSuggestions := DetectDuplicates(issues, lowConfig)

	if len(highSuggestions) >= len(lowSuggestions) && len(lowSuggestions) > 0 {
		t.Log("Lower threshold finds more matches as expected")
	}
}

func TestDetectDuplicates_MaxSuggestions(t *testing.T) {
	var issues []model.Issue
	for i := 0; i < 10; i++ {
		issues = append(issues, model.Issue{
			ID:     string(rune('A' + i)),
			Title:  "Implement user authentication system feature",
			Status: model.StatusOpen,
		})
	}

	config := DefaultDuplicateConfig()
	config.MaxSuggestions = 5

	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) > 5 {
		t.Errorf("DetectDuplicates() returned %d suggestions, want <= 5", len(suggestions))
	}
}

func TestDetectDuplicates_MinKeywords(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Fix it", Status: model.StatusOpen},
		{ID: "B", Title: "Fix it", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	config.MinKeywords = 3

	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) > 0 {
		t.Error("DetectDuplicates() should skip pairs with too few keywords")
	}
}

// ============================================================================
// DetectDuplicates Tests - Edge Cases
// ============================================================================

func TestDetectDuplicates_SingleIssue(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Single issue", Status: model.StatusOpen},
	}

	suggestions := DetectDuplicates(issues, DefaultDuplicateConfig())

	if len(suggestions) != 0 {
		t.Errorf("DetectDuplicates() with single issue returned %d suggestions, want 0", len(suggestions))
	}
}

func TestDetectDuplicates_EmptyIssues(t *testing.T) {
	suggestions := DetectDuplicates([]model.Issue{}, DefaultDuplicateConfig())

	if len(suggestions) != 0 {
		t.Errorf("DetectDuplicates() with empty input returned %d suggestions, want 0", len(suggestions))
	}
}

func TestDetectDuplicates_SuggestionStructure(t *testing.T) {
	issues := []model.Issue{
		{ID: "ISSUE-1", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "ISSUE-2", Title: "Implement user authentication system", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	suggestions := DetectDuplicates(issues, config)

	if len(suggestions) == 0 {
		t.Fatal("Expected at least one suggestion")
	}

	sug := suggestions[0]

	if sug.Type != SuggestionPotentialDuplicate {
		t.Errorf("Type = %v, want SuggestionPotentialDuplicate", sug.Type)
	}
	if sug.TargetBead == "" {
		t.Error("TargetBead should not be empty")
	}
	if sug.RelatedBead == "" {
		t.Error("RelatedBead should not be empty")
	}
	if sug.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if sug.Reason == "" {
		t.Error("Reason should not be empty")
	}
	if sug.Confidence < 0 || sug.Confidence > 1 {
		t.Errorf("Confidence = %v, should be between 0 and 1", sug.Confidence)
	}
	if sug.ActionCommand == "" {
		t.Error("ActionCommand should be provided for open issues")
	}
}

func TestDetectDuplicates_SortedBySimilarity(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "B", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "C", Title: "Implement user authentication features", Status: model.StatusOpen},
		{ID: "D", Title: "Authentication system implementation needed", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	config.JaccardThreshold = 0.4
	suggestions := DetectDuplicates(issues, config)

	for i := 1; i < len(suggestions); i++ {
		if suggestions[i].Confidence > suggestions[i-1].Confidence {
			t.Errorf("Suggestions not sorted by similarity: [%d].Confidence=%v > [%d].Confidence=%v",
				i, suggestions[i].Confidence, i-1, suggestions[i-1].Confidence)
		}
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestSortPairsBySimilarity(t *testing.T) {
	pairs := []DuplicatePair{
		{Issue1: "A", Issue2: "B", Similarity: 0.5},
		{Issue1: "C", Issue2: "D", Similarity: 0.9},
		{Issue1: "E", Issue2: "F", Similarity: 0.7},
	}

	sortPairsBySimilarity(pairs)

	expected := []float64{0.9, 0.7, 0.5}
	for i, p := range pairs {
		if p.Similarity != expected[i] {
			t.Errorf("pairs[%d].Similarity = %v, want %v", i, p.Similarity, expected[i])
		}
	}
}

func TestSortPairsBySimilarity_Empty(t *testing.T) {
	var pairs []DuplicatePair
	sortPairsBySimilarity(pairs)
}

func TestTruncateStringSlice(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		max     int
		wantLen int
	}{
		{"shorter than max", []string{"a", "b"}, 5, 2},
		{"equal to max", []string{"a", "b", "c"}, 3, 3},
		{"longer than max", []string{"a", "b", "c", "d", "e"}, 3, 3},
		{"empty slice", []string{}, 5, 0},
		{"max zero", []string{"a", "b"}, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateStringSlice(tt.input, tt.max)
			if len(result) != tt.wantLen {
				t.Errorf("truncateStringSlice() len = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

// ============================================================================
// DuplicateDetector Tests
// ============================================================================

func TestDuplicateDetector_Detect(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Implement user authentication system", Status: model.StatusOpen},
		{ID: "B", Title: "Implement user authentication system", Status: model.StatusOpen},
	}

	detector := NewDuplicateDetector(DefaultDuplicateConfig())
	suggestions := detector.Detect(issues)

	if len(suggestions) == 0 {
		t.Error("DuplicateDetector.Detect() found no duplicates")
	}
}

func TestDuplicateDetector_LastRun(t *testing.T) {
	detector := NewDuplicateDetector(DefaultDuplicateConfig())

	if !detector.LastRun().IsZero() {
		t.Error("LastRun() should be zero before first Detect()")
	}

	issues := []model.Issue{
		{ID: "A", Title: "Test issue", Status: model.StatusOpen},
	}

	before := time.Now()
	detector.Detect(issues)
	after := time.Now()

	lastRun := detector.LastRun()
	if lastRun.Before(before) || lastRun.After(after) {
		t.Errorf("LastRun() = %v, want between %v and %v", lastRun, before, after)
	}
}

func TestDuplicateDetector_CustomConfig(t *testing.T) {
	config := DuplicateConfig{
		JaccardThreshold:   0.9,
		MinKeywords:        5,
		IgnoreClosedVsOpen: false,
		MaxSuggestions:     10,
	}

	detector := NewDuplicateDetector(config)

	issues := []model.Issue{
		{ID: "A", Title: "Short", Status: model.StatusOpen},
		{ID: "B", Title: "Short", Status: model.StatusOpen},
	}

	suggestions := detector.Detect(issues)

	if len(suggestions) > 0 {
		t.Error("Custom config MinKeywords not respected")
	}
}

// ============================================================================
// DefaultDuplicateConfig Tests
// ============================================================================

func TestDefaultDuplicateConfig(t *testing.T) {
	config := DefaultDuplicateConfig()

	if config.JaccardThreshold != 0.7 {
		t.Errorf("JaccardThreshold = %v, want 0.7", config.JaccardThreshold)
	}
	if config.MinKeywords != 2 {
		t.Errorf("MinKeywords = %v, want 2", config.MinKeywords)
	}
	if !config.IgnoreClosedVsOpen {
		t.Error("IgnoreClosedVsOpen should be true by default")
	}
	if config.MaxSuggestions != 20 {
		t.Errorf("MaxSuggestions = %v, want 20", config.MaxSuggestions)
	}
}

// ============================================================================
// DuplicatePair Tests
// ============================================================================

func TestDuplicatePair_Fields(t *testing.T) {
	pair := DuplicatePair{
		Issue1:     "ISSUE-1",
		Issue2:     "ISSUE-2",
		Similarity: 0.85,
		Method:     "jaccard",
		Keywords:   []string{"authentication", "user", "login"},
	}

	if pair.Issue1 != "ISSUE-1" {
		t.Errorf("Issue1 = %v, want ISSUE-1", pair.Issue1)
	}
	if pair.Issue2 != "ISSUE-2" {
		t.Errorf("Issue2 = %v, want ISSUE-2", pair.Issue2)
	}
	if pair.Similarity != 0.85 {
		t.Errorf("Similarity = %v, want 0.85", pair.Similarity)
	}
	if pair.Method != "jaccard" {
		t.Errorf("Method = %v, want jaccard", pair.Method)
	}
	if len(pair.Keywords) != 3 {
		t.Errorf("Keywords len = %d, want 3", len(pair.Keywords))
	}
}

// ============================================================================
// Integration/Realistic Scenario Tests
// ============================================================================

func TestDetectDuplicates_RealisticScenario(t *testing.T) {
	issues := []model.Issue{
		{ID: "BUG-101", Title: "Login fails with special characters", Description: "User cannot login with special characters in password", Status: model.StatusOpen},
		{ID: "BUG-102", Title: "Login fails with special characters", Description: "User cannot login with special characters in password", Status: model.StatusOpen},
		{ID: "FEAT-201", Title: "Add OAuth2 integration for Google", Description: "Implement OAuth2 for Google login", Status: model.StatusOpen},
		{ID: "FEAT-202", Title: "Add OAuth2 integration for Facebook", Description: "Implement OAuth2 for Facebook login", Status: model.StatusOpen},
		{ID: "TASK-301", Title: "Update database schema", Description: "Add new columns for user preferences", Status: model.StatusOpen},
	}

	config := DefaultDuplicateConfig()
	config.JaccardThreshold = 0.4 // Lower threshold for this test
	suggestions := DetectDuplicates(issues, config)

	foundBugDupe := false
	foundFeatDupe := false

	for _, sug := range suggestions {
		if (sug.TargetBead == "BUG-101" && sug.RelatedBead == "BUG-102") ||
			(sug.TargetBead == "BUG-102" && sug.RelatedBead == "BUG-101") {
			foundBugDupe = true
		}
		if (sug.TargetBead == "FEAT-201" && sug.RelatedBead == "FEAT-202") ||
			(sug.TargetBead == "FEAT-202" && sug.RelatedBead == "FEAT-201") {
			foundFeatDupe = true
		}
	}

	if !foundBugDupe {
		t.Error("Should detect BUG-101 and BUG-102 as potential duplicates")
	}
	if !foundFeatDupe {
		t.Error("Should detect FEAT-201 and FEAT-202 as potential duplicates")
	}
}

func TestDetectDuplicates_LargeIssueSet(t *testing.T) {
	var issues []model.Issue
	for i := 0; i < 100; i++ {
		issues = append(issues, model.Issue{
			ID:     string(rune('A'+i/26)) + string(rune('A'+i%26)),
			Title:  "Unique issue number " + string(rune('0'+i%10)),
			Status: model.StatusOpen,
		})
	}

	issues[0].Title = "Implement authentication feature"
	issues[50].Title = "Implement authentication feature"

	config := DefaultDuplicateConfig()
	start := time.Now()
	suggestions := DetectDuplicates(issues, config)
	duration := time.Since(start)

	if duration > time.Second {
		t.Errorf("DetectDuplicates took %v for 100 issues, want < 1s", duration)
	}

	if len(suggestions) == 0 {
		t.Error("Should find at least one duplicate pair")
	}
}
