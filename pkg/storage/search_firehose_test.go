package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// These tests exercise the new firehose Ordered slice returned by SearchService.Search.
// They ensure:
// 1. Global chronological ordering across all datasources (newest first)
// 2. Deterministic ordering between repeated searches
// 3. Correct tie-breakers when CreatedAt timestamps collide (datasource asc, then ID asc)
// 4. Pagination uniqueness (no duplicates across pages) and continuity of chronological order

// TestFirehoseOrderedGlobalChronological verifies that the Ordered slice is globally
// sorted by CreatedAt descending across all datasources (true firehose view).
func TestFirehoseOrderedGlobalChronological(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	base := time.Now().Add(-1 * time.Hour)

	// Interleaved timestamps across datasources:
	// We'll purposely create blocks whose timestamps interleave when merged.
	testData := map[string][]core.Block{
		"ds_alpha": {
			&mockBlock{id: "a1", text: "test content", createdAt: base.Add(55 * time.Minute), source: "ds_alpha"},
			&mockBlock{id: "a2", text: "test content", createdAt: base.Add(40 * time.Minute), source: "ds_alpha"},
			&mockBlock{id: "a3", text: "test content", createdAt: base.Add(10 * time.Minute), source: "ds_alpha"},
		},
		"ds_beta": {
			&mockBlock{id: "b1", text: "test content", createdAt: base.Add(50 * time.Minute), source: "ds_beta"},
			&mockBlock{id: "b2", text: "test content", createdAt: base.Add(35 * time.Minute), source: "ds_beta"},
			&mockBlock{id: "b3", text: "test content", createdAt: base.Add(5 * time.Minute), source: "ds_beta"},
		},
		"ds_gamma": {
			&mockBlock{id: "g1", text: "test content", createdAt: base.Add(53 * time.Minute), source: "ds_gamma"},
			&mockBlock{id: "g2", text: "test content", createdAt: base.Add(15 * time.Minute), source: "ds_gamma"},
		},
	}

	setupTestData(t, manager, testData)

	svc := manager.GetSearchService()
	results, err := svc.Search(SearchParams{
		Query: "test",
		Page:  1,
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("firehose search failed: %v", err)
	}

	ordered := results.Ordered
	if len(ordered) != 8 {
		t.Fatalf("expected 8 total blocks in Ordered slice, got %d", len(ordered))
	}

	for i := 0; i < len(ordered)-1; i++ {
		if ordered[i].CreatedAt().Before(ordered[i+1].CreatedAt()) {
			t.Errorf("global chronological violation at index %d: %v BEFORE %v",
				i, ordered[i].CreatedAt(), ordered[i+1].CreatedAt())
		}
	}
}

// TestFirehoseOrderedDeterministic ensures two identical searches produce identical Ordered sequences.
func TestFirehoseOrderedDeterministic(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	base := time.Now()

	testData := map[string][]core.Block{
		"news": {
			&mockBlock{id: "n1", text: "test item", createdAt: base.Add(-1 * time.Minute), source: "news"},
			&mockBlock{id: "n2", text: "test item", createdAt: base.Add(-10 * time.Minute), source: "news"},
		},
		"blog": {
			&mockBlock{id: "b1", text: "test item", createdAt: base.Add(-2 * time.Minute), source: "blog"},
			&mockBlock{id: "b2", text: "test item", createdAt: base.Add(-11 * time.Minute), source: "blog"},
		},
		"social": {
			&mockBlock{id: "s1", text: "test item", createdAt: base.Add(-3 * time.Minute), source: "social"},
			&mockBlock{id: "s2", text: "test item", createdAt: base.Add(-12 * time.Minute), source: "social"},
		},
	}

	setupTestData(t, manager, testData)

	svc := manager.GetSearchService()

	getIDs := func() []string {
		res, err := svc.Search(SearchParams{Query: "test", Page: 1, Limit: 10})
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		ids := make([]string, len(res.Ordered))
		for i, b := range res.Ordered {
			ids[i] = fmt.Sprintf("%s|%s|%d", b.ID(), b.Source(), b.CreatedAt().UnixNano())
		}
		return ids
	}

	first := getIDs()
	second := getIDs()

	if len(first) != len(second) {
		t.Fatalf("different lengths: first=%d second=%d", len(first), len(second))
	}

	for i := range first {
		if first[i] != second[i] {
			t.Errorf("determinism failure at index %d: %s != %s", i, first[i], second[i])
		}
	}
}

// TestFirehoseOrderedTieBreakers verifies deterministic ordering when CreatedAt timestamps are identical.
// Expected ordering: datasource ascending, then block ID ascending.
func TestFirehoseOrderedTieBreakers(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	// Fixed timestamp
	ts := time.Now().Add(-30 * time.Minute)

	// All blocks share the same CreatedAt - rely purely on tie-breakers.
	testData := map[string][]core.Block{
		"gamma": {
			&mockBlock{id: "g2", text: "test tiebreak", createdAt: ts, source: "gamma"},
			&mockBlock{id: "g1", text: "test tiebreak", createdAt: ts, source: "gamma"},
		},
		"alpha": {
			&mockBlock{id: "a2", text: "test tiebreak", createdAt: ts, source: "alpha"},
			&mockBlock{id: "a1", text: "test tiebreak", createdAt: ts, source: "alpha"},
		},
		"beta": {
			&mockBlock{id: "b1", text: "test tiebreak", createdAt: ts, source: "beta"},
			&mockBlock{id: "b2", text: "test tiebreak", createdAt: ts, source: "beta"},
		},
	}

	setupTestData(t, manager, testData)

	svc := manager.GetSearchService()
	results, err := svc.Search(SearchParams{
		Query: "tiebreak",
		Page:  1,
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	var ids []string
	for _, b := range results.Ordered {
		ids = append(ids, fmt.Sprintf("%s:%s", b.Source(), b.ID()))
	}

	// Expected ordering:
	// datasource asc: alpha, beta, gamma
	// within each datasource ID asc
	expected := []string{
		"alpha:a1", "alpha:a2",
		"beta:b1", "beta:b2",
		"gamma:g1", "gamma:g2",
	}

	if len(ids) != len(expected) {
		t.Fatalf("expected %d ordered blocks, got %d", len(expected), len(ids))
	}

	for i := range expected {
		if ids[i] != expected[i] {
			t.Errorf("tie-break ordering mismatch at %d: got %s expected %s", i, ids[i], expected[i])
		}
	}
}

// TestFirehosePaginationUniqueness ensures uniqueness & chronological continuity across pages.
func TestFirehosePaginationUniqueness(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	base := time.Now()

	// Construct 3 datasources with interleaved times: total 45 blocks
	// We'll request page size 10 and iterate pages until empty.
	dsCount := 3
	blocksPerDatasource := 15
	testData := make(map[string][]core.Block)

	for d := 0; d < dsCount; d++ {
		dsName := fmt.Sprintf("ds_%d", d)
		testData[dsName] = make([]core.Block, blocksPerDatasource)
		for i := 0; i < blocksPerDatasource; i++ {
			// Newest earlier: subtract minutes
			testData[dsName][i] = &mockBlock{
				id:        fmt.Sprintf("%s_b_%02d", dsName, i),
				text:      "pagination uniqueness content",
				createdAt: base.Add(-time.Duration(i*dsCount+d) * time.Minute), // interleave by datasource
				source:    dsName,
			}
		}
	}

	setupTestData(t, manager, testData)
	svc := manager.GetSearchService()

	seen := make(map[string]bool)
	page := 1
	pageSize := 10
	var lastTimestamp *time.Time
	totalSeen := 0
	expectedTotal := dsCount * blocksPerDatasource

	for {
		res, err := svc.Search(SearchParams{
			Query: "pagination",
			Page:  page,
			Limit: pageSize,
		})
		if err != nil {
			t.Fatalf("page %d search failed: %v", page, err)
		}

		if len(res.Ordered) == 0 {
			break
		}

		// Validate ordering inside the page
		for i, b := range res.Ordered {
			// Chronological inside page
			if i > 0 && res.Ordered[i-1].CreatedAt().Before(b.CreatedAt()) {
				t.Errorf("page %d local ordering violation at index %d: %v BEFORE %v",
					page, i-1, res.Ordered[i-1].CreatedAt(), b.CreatedAt())
			}

			// Cross-page continuity: lastTimestamp should be >= current (descending order)
			if lastTimestamp != nil && lastTimestamp.Before(b.CreatedAt()) {
				t.Errorf("cross-page ordering violation: previous page oldest %v BEFORE new page block %v",
					*lastTimestamp, b.CreatedAt())
			}

			if seen[b.ID()] {
				t.Errorf("duplicate block ID across pages: %s", b.ID())
			}
			seen[b.ID()] = true
			totalSeen++
		}

		// Store oldest timestamp of this page (last element) for next page continuity check
		oldest := res.Ordered[len(res.Ordered)-1].CreatedAt()
		lastTimestamp = &oldest

		page++
	}

	if totalSeen != expectedTotal {
		t.Errorf("expected to see %d unique blocks across pages, got %d", expectedTotal, totalSeen)
	}
}
