package model

import "testing"

// ===== Normalize =====

func TestPageQuery_Normalize_ClampsNegativePage(t *testing.T) {
	cases := []struct {
		name  string
		input PageQuery
		want  PageQuery
	}{
		{"page=0 -> 1", PageQuery{Page: 0, Size: 20}, PageQuery{Page: 1, Size: 20}},
		{"page=-5 -> 1", PageQuery{Page: -5, Size: 20}, PageQuery{Page: 1, Size: 20}},
		{"size=0 -> 20", PageQuery{Page: 1, Size: 0}, PageQuery{Page: 1, Size: DefaultPageSize}},
		{"size=-1 -> 20", PageQuery{Page: 1, Size: -1}, PageQuery{Page: 1, Size: DefaultPageSize}},
		{"size=300 -> 200", PageQuery{Page: 2, Size: 300}, PageQuery{Page: 2, Size: MaxPageSize}},
		{"all-valid", PageQuery{Page: 3, Size: 50}, PageQuery{Page: 3, Size: 50}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.input.Normalize()
			if got != c.want {
				t.Errorf("Normalize(%+v) = %+v, want %+v", c.input, got, c.want)
			}
		})
	}
}

// ===== Offset =====

func TestPageQuery_Offset_ComputesCorrectly(t *testing.T) {
	cases := []struct {
		name  string
		query PageQuery
		want  int
	}{
		{"page=1,size=20 -> 0", PageQuery{Page: 1, Size: 20}, 0},
		{"page=2,size=20 -> 20", PageQuery{Page: 2, Size: 20}, 20},
		{"page=3,size=10 -> 20", PageQuery{Page: 3, Size: 10}, 20},
		{"page=1,size=200 -> 0", PageQuery{Page: 1, Size: 200}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.query.Offset()
			if got != c.want {
				t.Errorf("Offset() = %d, want %d", got, c.want)
			}
		})
	}
}

// ===== NewPageResult =====

func TestNewPageResult_PagesRounding(t *testing.T) {
	cases := []struct {
		name      string
		records   any
		total     int64
		query     PageQuery
		wantPages int64
		wantSize  int
		wantCurr  int
	}{
		{"total=0", "anything", 0, PageQuery{Page: 1, Size: 20}, 0, 20, 1},
		{"exact-division", "data", 10, PageQuery{Page: 2, Size: 5}, 2, 5, 2},
		{"not-divisible", "items", 11, PageQuery{Page: 1, Size: 10}, 2, 10, 1},
		{"single-page", "small", 3, PageQuery{Page: 1, Size: 10}, 1, 10, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewPageResult(c.records, c.total, c.query)
			if r.Pages != c.wantPages {
				t.Errorf("Pages = %d, want %d", r.Pages, c.wantPages)
			}
			if r.Size != c.wantSize {
				t.Errorf("Size = %d, want %d", r.Size, c.wantSize)
			}
			if r.Current != c.wantCurr {
				t.Errorf("Current = %d, want %d", r.Current, c.wantCurr)
			}
			if r.Records != c.records {
				t.Errorf("Records pointer changed")
			}
		})
	}
}

// ===== 原 query_term_mapping_test.go TestBuildPageResult_Math 迁移 =====

func TestNewPageResult_MigratedFromQueryTermMapping(t *testing.T) {
	r := NewPageResult([]string{"a", "b"}, 25, PageQuery{Page: 3, Size: 10})
	if r.Total != 25 || r.Current != 3 || r.Size != 10 {
		t.Errorf("basic fields: Total=%d Current=%d Size=%d", r.Total, r.Current, r.Size)
	}
	if r.Pages != 3 { // ceil(25/10)
		t.Errorf("Pages = %d, want 3", r.Pages)
	}
	if len(r.Records.([]string)) != 2 {
		t.Errorf("Records len = %d, want 2", len(r.Records.([]string)))
	}
}
