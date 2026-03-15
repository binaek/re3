package re3

import (
	"context"
	"reflect"
	"testing"
)

func TestInjectCaptureTags(t *testing.T) {
	// GroupNode(1, a) -> Concat(Tag(1,start), Concat(a, Tag(1,end)))
	ctx := context.Background()
	a := newLiteralNode(ctx, 'a')
	group := newGroupNode(ctx, 1, a)
	tagged := injectCaptureTags(ctx, group)

	concat, ok := tagged.(*concatNode)
	if !ok {
		t.Fatalf("injectCaptureTags(Group(1,a)) should return concatNode, got %T", tagged)
	}
	tagStart, ok := concat.Left.(*tagNode)
	if !ok || tagStart.Id != 1 || !tagStart.IsStart {
		t.Errorf("left of Concat should be Tag(1,start), got %v", concat.Left)
	}
	inner, ok := concat.Right.(*concatNode)
	if !ok {
		t.Fatalf("right of Concat should be Concat(a, Tag), got %T", concat.Right)
	}
	if !inner.Left.Equals(a) {
		t.Errorf("inner Concat left should be Lit('a'), got %v", inner.Left)
	}
	tagEnd, ok := inner.Right.(*tagNode)
	if !ok || tagEnd.Id != 1 || tagEnd.IsStart {
		t.Errorf("inner Concat right should be Tag(1,end), got %v", inner.Right)
	}
}

func TestStepTDFA_Basic(t *testing.T) {
	// stepTDFA(Lit('a'), 'a') -> one config: NextNode=Empty, Tags=nil
	ctx := context.Background()
	lit := newLiteralNode(ctx, 'a')
	configs := stepTDFA(ctx, lit, 'a', matchContext{})
	if len(configs) != 1 {
		t.Fatalf("stepTDFA(Lit('a'), 'a') want 1 config, got %d", len(configs))
	}
	if _, ok := configs[0].NextNode.(*emptyNode); !ok {
		t.Errorf("NextNode want emptyNode, got %T", configs[0].NextNode)
	}
	if configs[0].Tags != nil {
		t.Errorf("Tags want nil, got %v", configs[0].Tags)
	}

	// stepTDFA(Lit('a'), 'b') -> one config: NextNode=False
	configs = stepTDFA(ctx, lit, 'b', matchContext{})
	if len(configs) != 1 {
		t.Fatalf("stepTDFA(Lit('a'), 'b') want 1 config, got %d", len(configs))
	}
	if _, ok := configs[0].NextNode.(*falseNode); !ok {
		t.Errorf("NextNode want falseNode, got %T", configs[0].NextNode)
	}
}

func TestStepTDFA_UnionDisambiguation(t *testing.T) {
	// Union(Concat(Tag(1,start), a), Concat(Tag(2,start), a)). step 'a' -> two configs;
	// left branch yields (Empty, [Tag1]) among others, right yields (Empty, [Tag2]).
	// Configs order: left configs first, then right. So first Empty config has Tag1.
	// TDFA compiler takes configs[0] when multiple configs have same NextNode -> leftmost wins.
	ctx := context.Background()
	tag1a := newConcatNode(ctx, newTagNode(ctx, 1, true), newLiteralNode(ctx, 'a'))
	tag2a := newConcatNode(ctx, newTagNode(ctx, 2, true), newLiteralNode(ctx, 'a'))
	u := newUnionNodeOrdered(ctx, tag1a, tag2a)
	configs := stepTDFA(ctx, u, 'a', matchContext{})
	if len(configs) < 2 {
		t.Fatalf("stepTDFA(Union(Concat(Tag1,a), Concat(Tag2,a)), 'a') want at least 2 configs, got %d", len(configs))
	}
	// Find configs that have EmptyNode as next (accepting). Left branch gives one with Tags [Tag1];
	// right gives one with Tags [Tag2]. First in slice should be from left (Tag1).
	var emptyConfigs []tdfaConfig
	for _, c := range configs {
		if _, ok := c.NextNode.(*emptyNode); ok {
			emptyConfigs = append(emptyConfigs, c)
		}
	}
	if len(emptyConfigs) < 1 {
		t.Fatal("expected at least one config with EmptyNode")
	}
	first := emptyConfigs[0]
	if len(first.Tags) != 1 || first.Tags[0].Id != 1 || !first.Tags[0].IsStart {
		t.Errorf("POSIX leftmost: first Empty config should have Tag(1,start), got %v", first.Tags)
	}
}

func TestCountCaptureGroups(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		ast  node
		want int
	}{
		{"no group", newLiteralNode(ctx, 'a'), 0},
		{"one group", newGroupNode(ctx, 1, newLiteralNode(ctx, 'a')), 1},
		{"two groups", newConcatNode(ctx,
			newGroupNode(ctx, 1, newLiteralNode(ctx, 'a')),
			newGroupNode(ctx, 2, newLiteralNode(ctx, 'b')),
		), 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := countCaptureGroups(tc.ast)
			if got != tc.want {
				t.Errorf("countCaptureGroups = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRunTDFA_AcceptingSetsCaptureZero(t *testing.T) {
	// (a) on "a": tagged root = Concat(Tag(1,start), Concat(a, Tag(1,end))). Run on "a", accept at end.
	re := MustCompile("(a)").(*regexpImpl)
	if re.CaptureCount != 1 {
		t.Fatalf("CaptureCount want 1, got %d", re.CaptureCount)
	}
	ctx := context.Background()
	tagged := injectCaptureTags(ctx, re.forward.root)
	configs := stepTDFA(ctx, tagged, 'a', matchContext{})
	if len(configs) == 0 {
		t.Fatalf("stepTDFA(taggedRoot, 'a') returned 0 configs (r might be wrong in getNextState)")
	}
	loc := re.FindStringIndex("a")
	if loc == nil {
		t.Fatal("FindStringIndex((a), \"a\") returned nil")
	}
	if re.forwardTDFA == nil {
		re.forwardTDFA = newLazyTDFA(ctx, tagged, re.minterms, re.CaptureCount)
	}
	capture := re.forwardTDFA.runTDFA(ctx, "a")
	if capture == nil {
		t.Fatal("runTDFA(\"a\") returned nil")
	}
	got := re.FindStringSubmatch("a")
	want := []string{"a", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindStringSubmatch((a), \"a\") = %v, want %v", got, want)
	}
}
