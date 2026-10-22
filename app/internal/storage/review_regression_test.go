package storage

import (
	"testing"

	"litkit/internal/model"
)

func TestRound2_RenamedTitleDoesNotMatchOldTitle(t *testing.T) {
	s := newTestStore(t)
	p := model.Paper{Title: "Old title", PMID: "42", Abstract: "original"}
	p.ID = p.ComputeID()
	key, _, err := s.UpsertPaper(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Title = "Corrected title"
	if _, _, err := s.UpsertPaper(p); err != nil {
		t.Fatal(err)
	}
	other := model.Paper{Title: "Old title", PMID: "99", Abstract: "different paper"}
	other.ID = other.ComputeID()
	otherKey, inserted, err := s.UpsertPaper(other)
	if err != nil {
		t.Fatal(err)
	}
	if key == otherKey || !inserted {
		t.Fatalf("stale title key overwrote corrected paper: key=%s, inserted=%v", otherKey, inserted)
	}
}

func TestRefsTransactionsRollbackOnFailure(t *testing.T) {
	s := newTestStore(t)
	key, _, err := s.UpsertPaper(model.Paper{Title: "Study", Abstract: "Evidence"})
	if err != nil {
		t.Fatal(err)
	}
	old := model.PaperRef{CiteKey: key, SentenceHash: "old", Manuscript: "doc.md", Sentence: "Old"}
	if err := s.AddRef(old); err != nil {
		t.Fatal(err)
	}
	refs := []model.PaperRef{
		{CiteKey: key, SentenceHash: "new", Manuscript: "doc.md", Sentence: "New"},
		{CiteKey: "missing-key", SentenceHash: "missing", Manuscript: "doc.md"},
	}
	for _, replace := range []bool{false, true} {
		var err error
		if replace {
			err = s.ReplaceRefsByManuscript("doc.md", refs)
		} else {
			err = s.AddRefs(refs)
		}
		if err == nil {
			t.Fatal("missing key must fail")
		}
		got, err := s.GetRefsByManuscript("doc.md")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].SentenceHash != "old" {
			t.Fatalf("failed operation changed old refs: %+v", got)
		}
	}
}
