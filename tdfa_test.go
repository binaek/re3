package re3

import (
	"reflect"
	"testing"
)

func TestInjectCaptureTags(t *testing.T) {
	// GroupNode(1, a) -> Concat(Tag(1,start), Concat(a, Tag(1,end)))
	a := &LiteralNode{Value: 'a'}
	group := &GroupNode{GroupID: 1, Child: a}
	tagged := InjectCaptureTags(group)

	concat, ok := tagged.(*ConcatNode)
	if !ok {
		t.Fatalf("InjectCaptureTags(Group(1,a)) should return ConcatNode, got %T", tagged)
	}
	tagStart, ok := concat.Left.(*TagNode)
	if !ok || tagStart.Id != 1 || !tagStart.IsStart {
		t.Errorf("left of Concat should be Tag(1,start), got %v", concat.Left)
	}
	inner, ok := concat.Right.(*ConcatNode)
	if !ok {
		t.Fatalf("right of Concat should be Concat(a, Tag), got %T", concat.Right)
	}
	if !inner.Left.Equals(a) {
		t.Errorf("inner Concat left should be Lit('a'), got %v", inner.Left)
	}
	tagEnd, ok := inner.Right.(*TagNode)
	if !ok || tagEnd.Id != 1 || tagEnd.IsStart {
		t.Errorf("inner Concat right should be Tag(1,end), got %v", inner.Right)
	}
}

func TestStepTDFA_Basic(t *testing.T) {
	// stepTDFA(Lit('a'), 'a') -> one config: NextNode=Empty, Tags=nil
	lit := &LiteralNode{Value: 'a'}
	configs := stepTDFA(lit, 'a')
	if len(configs) != 1 {
		t.Fatalf("stepTDFA(Lit('a'), 'a') want 1 config, got %d", len(configs))
	}
	if _, ok := configs[0].NextNode.(*EmptyNode); !ok {
		t.Errorf("NextNode want EmptyNode, got %T", configs[0].NextNode)
	}
	if configs[0].Tags != nil {
		t.Errorf("Tags want nil, got %v", configs[0].Tags)
	}

	// stepTDFA(Lit('a'), 'b') -> one config: NextNode=False
	configs = stepTDFA(lit, 'b')
	if len(configs) != 1 {
		t.Fatalf("stepTDFA(Lit('a'), 'b') want 1 config, got %d", len(configs))
	}
	if _, ok := configs[0].NextNode.(*FalseNode); !ok {
		t.Errorf("NextNode want FalseNode, got %T", configs[0].NextNode)
	}
}

func TestStepTDFA_UnionDisambiguation(t *testing.T) {
	// Union(Concat(Tag(1,start), a), Concat(Tag(2,start), a)). step 'a' -> two configs;
	// left branch yields (Empty, [Tag1]) among others, right yields (Empty, [Tag2]).
	// Configs order: left configs first, then right. So first Empty config has Tag1.
	// TDFA compiler takes configs[0] when multiple configs have same NextNode -> leftmost wins.
	tag1a := &ConcatNode{
		Left:  &TagNode{Id: 1, IsStart: true},
		Right: &LiteralNode{Value: 'a'},
	}
	tag2a := &ConcatNode{
		Left:  &TagNode{Id: 2, IsStart: true},
		Right: &LiteralNode{Value: 'a'},
	}
	u := &UnionNode{Left: tag1a, Right: tag2a}
	configs := stepTDFA(u, 'a')
	if len(configs) < 2 {
		t.Fatalf("stepTDFA(Union(Concat(Tag1,a), Concat(Tag2,a)), 'a') want at least 2 configs, got %d", len(configs))
	}
	// Find configs that have EmptyNode as next (accepting). Left branch gives one with Tags [Tag1];
	// right gives one with Tags [Tag2]. First in slice should be from left (Tag1).
	var emptyConfigs []tdfaConfig
	for _, c := range configs {
		if _, ok := c.NextNode.(*EmptyNode); ok {
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
	tests := []struct {
		name string
		ast  Node
		want int
	}{
		{"no group", &LiteralNode{Value: 'a'}, 0},
		{"one group", &GroupNode{GroupID: 1, Child: &LiteralNode{Value: 'a'}}, 1},
		{"two groups", NewConcatNode(
			&GroupNode{GroupID: 1, Child: &LiteralNode{Value: 'a'}},
			&GroupNode{GroupID: 2, Child: &LiteralNode{Value: 'b'}},
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
	tagged := InjectCaptureTags(re.forward.root)
	configs := stepTDFA(tagged, 'a')
	if len(configs) == 0 {
		t.Fatalf("stepTDFA(taggedRoot, 'a') returned 0 configs (r might be wrong in getNextState)")
	}
	loc := re.FindStringIndex("a")
	if loc == nil {
		t.Fatal("FindStringIndex((a), \"a\") returned nil")
	}
	if re.forwardTDFA == nil {
		re.forwardTDFA = newLazyTDFA(tagged, re.minterms, re.CaptureCount)
	}
	capture := re.forwardTDFA.runTDFA("a")
	if capture == nil {
		t.Fatal("runTDFA(\"a\") returned nil")
	}
	got := re.FindStringSubmatch("a")
	want := []string{"a", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindStringSubmatch((a), \"a\") = %v, want %v", got, want)
	}
}
