package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestUpstreamWalk(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/walk.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sourceMapFixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 59 {
		t.Fatal(len(cases))
	}
	for i, tt := range cases {
		t.Run(fmt.Sprintf("%d/%s", i, tt.Test), func(t *testing.T) {
			var input, output []upstreamNode
			var steps []struct {
				Phase          string
				Before, After  upstreamNode
				Parent         *upstreamNode
				Depth, Index   int
				Siblings, Path []upstreamNode
				Action         struct {
					Kind  WalkKind
					Nodes []upstreamNode
				}
			}
			json.Unmarshal(tt.Args[0], &input)
			json.Unmarshal(tt.Args[1], &steps)
			json.Unmarshal(tt.Result, &output)
			next := 0
			hook := func(phase string) func(*Node, *VisitContext) WalkAction {
				return func(node *Node, ctx *VisitContext) WalkAction {
					if next >= len(steps) {
						t.Fatalf("unexpected %s %s", phase, node.Kind)
					}
					step := steps[next]
					next++
					check := func(field string, actual, want any) {
						if !reflect.DeepEqual(actual, want) {
							t.Fatalf("step %d %s: got %#v, want %#v", next, field, actual, want)
						}
					}
					check("phase", phase, step.Phase)
					check("node", node, decodeWalkNodes([]upstreamNode{step.Before})[0])
					check("depth", ctx.Depth, step.Depth)
					check("index", ctx.Index, step.Index)
					check("siblings", ctx.Siblings, decodeWalkNodes(step.Siblings))
					check("path", ctx.Path(), decodeWalkNodes(step.Path))
					var parent *Node
					if step.Parent != nil {
						parent = decodeWalkNodes([]upstreamNode{*step.Parent})[0]
					}
					check("parent", ctx.Parent, parent)
					if ctx.Siblings[ctx.Index] != node {
						t.Fatal("node identity lost")
					}
					// Replay only mutations made by the original callback.
					if !reflect.DeepEqual(step.Before, step.After) {
						*node = *decodeWalkNodes([]upstreamNode{step.After})[0]
					}
					return WalkAction{Kind: step.Action.Kind, Nodes: decodeWalkNodes(step.Action.Nodes)}
				}
			}
			got, err := Walk(decodeWalkNodes(input), WalkHooks{Enter: hook("enter"), Exit: hook("exit")})
			if err != nil || next != len(steps) || !reflect.DeepEqual(got, decodeWalkNodes(output)) {
				t.Fatalf("walk: %v, steps %d/%d, output %#v", err, next, len(steps), got)
			}
		})
	}
}

func TestWalkActionsAndDepth(t *testing.T) {
	for _, phase := range []string{"enter", "exit"} {
		for _, kind := range []WalkKind{WalkSkip, 255} {
			hooks := WalkHooks{}
			hook := func(*Node, *VisitContext) WalkAction { return WalkAction{Kind: kind} }
			if phase == "enter" {
				hooks.Enter = hook
			} else {
				hooks.Exit = hook
			}
			_, err := Walk([]*Node{{Kind: "a"}}, hooks)
			if kind == WalkSkip && phase == "enter" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "in "+phase) {
				t.Fatal(err)
			}
		}
	}
	root := &Node{Kind: "root"}
	for i := 0; i < 100000; i++ {
		root = &Node{Kind: "a", Nodes: []*Node{root}}
	}
	count := 0
	_, err := Walk([]*Node{root}, WalkHooks{Exit: func(node *Node, ctx *VisitContext) WalkAction {
		if ctx.Depth == 100000 && len(ctx.Path()) != ctx.Depth {
			t.Fatal("wrong path")
		}
		count++
		return WalkAction{}
	}})
	if err != nil || count != 100001 {
		t.Fatal(count, err)
	}
	if got, err := Walk(nil, WalkHooks{}); got != nil || err != nil {
		t.Fatal(got, err)
	}
	nodes := []*Node{{Kind: "a"}, {Kind: "b"}}
	got, err := Walk(nodes, WalkHooks{})
	if err != nil || !slices.Equal(nodes, got) {
		t.Fatal(got, err)
	}
}

func BenchmarkWalk(b *testing.B) {
	nodes := []*Node{{Kind: "rule", Nodes: []*Node{{Kind: "declaration"}, {Kind: "declaration"}}}}
	for b.Loop() {
		_, _ = Walk(nodes, WalkHooks{Enter: func(*Node, *VisitContext) WalkAction { return WalkAction{} }})
	}
}

func decodeWalkNodes(input []upstreamNode) []*Node {
	nodes := convertUpstreamNodes(input, "")
	for i, n := range input {
		if n.Nodes != nil {
			nodes[i].Nodes = decodeWalkNodes(n.Nodes)
		}
	}
	return nodes
}
