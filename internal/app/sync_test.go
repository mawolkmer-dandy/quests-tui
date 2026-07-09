package app

import (
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// stackModel builds a bare Model with just the prStatus cache populated, for
// testing prStack ordering in isolation.
func stackModel(sts ...PRStatus) *Model {
	m := &Model{prStatus: map[string]PRStatus{}}
	for _, st := range sts {
		m.prStatus[st.Code] = st
	}
	return m
}

func codesAndDepths(nodes []prStackNode) ([]string, []int) {
	codes := make([]string, len(nodes))
	depths := make([]int, len(nodes))
	for i, n := range nodes {
		codes[i] = n.link.Code
		depths[i] = n.depth
	}
	return codes, depths
}

func TestPRStack(t *testing.T) {
	t.Run("lone PR is a single root", func(t *testing.T) {
		m := stackModel(PRStatus{Code: "#1", BaseRef: "main", HeadRef: "feat"})
		nodes := m.prStack([]model.PRLink{{Code: "#1"}})
		codes, depths := codesAndDepths(nodes)
		if len(nodes) != 1 || codes[0] != "#1" || depths[0] != 0 {
			t.Fatalf("got codes=%v depths=%v", codes, depths)
		}
		if nodes[0].stacked {
			t.Fatalf("a lone PR must not be marked stacked")
		}
	})

	t.Run("child directly below parent", func(t *testing.T) {
		// #1 targets main (root), #2's base == #1's head (child).
		m := stackModel(
			PRStatus{Code: "#1", BaseRef: "main", HeadRef: "a"},
			PRStatus{Code: "#2", BaseRef: "a", HeadRef: "b"},
		)
		// Link order deliberately child-first, to prove ordering is by tree.
		nodes := m.prStack([]model.PRLink{{Code: "#2"}, {Code: "#1"}})
		codes, depths := codesAndDepths(nodes)
		if len(nodes) != 2 || codes[0] != "#1" || codes[1] != "#2" {
			t.Fatalf("got codes=%v, want [#1 #2]", codes)
		}
		if depths[0] != 0 || depths[1] != 1 {
			t.Fatalf("got depths=%v, want [0 1]", depths)
		}
		if !nodes[0].stacked || !nodes[1].stacked {
			t.Fatalf("both PRs in a stack must be marked stacked")
		}
	})

	t.Run("three-deep stack", func(t *testing.T) {
		m := stackModel(
			PRStatus{Code: "#1", BaseRef: "main", HeadRef: "a"},
			PRStatus{Code: "#2", BaseRef: "a", HeadRef: "b"},
			PRStatus{Code: "#3", BaseRef: "b", HeadRef: "c"},
		)
		nodes := m.prStack([]model.PRLink{{Code: "#3"}, {Code: "#1"}, {Code: "#2"}})
		codes, depths := codesAndDepths(nodes)
		wantC := []string{"#1", "#2", "#3"}
		wantD := []int{0, 1, 2}
		for i := range wantC {
			if codes[i] != wantC[i] || depths[i] != wantD[i] {
				t.Fatalf("got codes=%v depths=%v, want %v %v", codes, depths, wantC, wantD)
			}
		}
	})

	t.Run("independent PRs are separate roots", func(t *testing.T) {
		m := stackModel(
			PRStatus{Code: "#1", BaseRef: "main", HeadRef: "a"},
			PRStatus{Code: "#2", BaseRef: "main", HeadRef: "b"},
		)
		nodes := m.prStack([]model.PRLink{{Code: "#1"}, {Code: "#2"}})
		_, depths := codesAndDepths(nodes)
		if len(nodes) != 2 || depths[0] != 0 || depths[1] != 0 {
			t.Fatalf("got depths=%v, want two roots [0 0]", depths)
		}
		if nodes[0].stacked || nodes[1].stacked {
			t.Fatalf("independent PRs must not be marked stacked")
		}
	})

	t.Run("unsynced set renders flat in link order", func(t *testing.T) {
		m := stackModel() // no statuses cached
		nodes := m.prStack([]model.PRLink{{Code: "#1"}, {Code: "#2"}})
		codes, depths := codesAndDepths(nodes)
		if len(nodes) != 2 || codes[0] != "#1" || codes[1] != "#2" || depths[0] != 0 || depths[1] != 0 {
			t.Fatalf("got codes=%v depths=%v, want flat roots", codes, depths)
		}
	})

	t.Run("ref cycle does not hang and renders all", func(t *testing.T) {
		m := stackModel(
			PRStatus{Code: "#1", BaseRef: "b", HeadRef: "a"},
			PRStatus{Code: "#2", BaseRef: "a", HeadRef: "b"},
		)
		nodes := m.prStack([]model.PRLink{{Code: "#1"}, {Code: "#2"}})
		if len(nodes) != 2 {
			t.Fatalf("got %d nodes, want 2 (cycle must still render every PR)", len(nodes))
		}
	})
}
