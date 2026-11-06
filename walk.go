package rocketwind

import (
	"fmt"
	"slices"
)

// WalkKind controls traversal after a visitor returns. Its zero value continues.
type WalkKind uint8

const (
	WalkContinue WalkKind = iota
	WalkSkip
	WalkStop
	WalkReplace
	WalkReplaceSkip
	WalkReplaceStop
)

// WalkAction replaces the current node when Kind is one of the replacement
// actions. An empty Nodes slice removes it. Replacement nodes are visited only
// for WalkReplace during entry; replacements during exit are never revisited.
type WalkAction struct {
	Kind  WalkKind
	Nodes []*Node
}

type WalkHooks struct {
	Enter func(*Node, *VisitContext) WalkAction
	Exit  func(*Node, *VisitContext) WalkAction
}

// VisitContext describes the current position. It is reused between callbacks;
// retain Path's result if you need an independently owned ancestor list.
type VisitContext struct {
	Parent       *Node
	Depth, Index int
	Siblings     []*Node
	stack        []walkFrame
}

// Path returns ancestors from the root to Parent, excluding the current node.
func (ctx *VisitContext) Path() []*Node {
	path := make([]*Node, 0, ctx.Depth)
	for _, frame := range ctx.stack {
		if frame.parent != nil {
			path = append(path, frame.parent)
		}
	}
	return path
}

type walkFrame struct {
	nodes  *[]*Node
	index  int
	parent *Node
}

// Walk visits and edits a CSS tree in place, without recursion. Always retain
// the returned root slice: replacing root nodes can change its storage or length.
// Enter runs before children, Exit after them. Skip skips children but still
// calls Exit; Stop ends the entire traversal immediately. Skip is invalid in
// Exit. Nil hooks continue by default. Callers own the tree while walking it.
func Walk(nodes []*Node, hooks WalkHooks) ([]*Node, error) {
	var local [16]walkFrame
	ctx := VisitContext{stack: append(local[:0], walkFrame{nodes: &nodes})}
	for len(ctx.stack) > 0 {
		frame := &ctx.stack[len(ctx.stack)-1]
		if frame.index >= len(*frame.nodes) {
			ctx.stack = ctx.stack[:len(ctx.stack)-1]
			continue
		}
		exiting := frame.index < 0
		index := frame.index
		if exiting {
			index = ^index
		}
		ctx.Parent, ctx.Depth = frame.parent, len(ctx.stack)-1
		ctx.Index, ctx.Siblings = index, *frame.nodes
		node := (*frame.nodes)[index]
		action := WalkAction{}
		if exiting {
			if hooks.Exit != nil {
				action = hooks.Exit(node, &ctx)
			}
		} else if hooks.Enter != nil {
			action = hooks.Enter(node, &ctx)
		}
		switch action.Kind {
		case WalkContinue, WalkSkip:
			if exiting {
				if action.Kind == WalkSkip {
					return nodes, fmt.Errorf("Invalid `WalkAction.Skip` in exit.")
				}
				frame.index = index + 1
			} else {
				frame.index = ^index
				if action.Kind == WalkContinue && len(node.Nodes) > 0 {
					ctx.stack = append(ctx.stack, walkFrame{nodes: &node.Nodes, parent: node})
				}
			}
		case WalkStop:
			return nodes, nil
		case WalkReplace, WalkReplaceSkip, WalkReplaceStop:
			*frame.nodes = slices.Replace(*frame.nodes, index, index+1, action.Nodes...)
			if action.Kind == WalkReplaceStop {
				return nodes, nil
			}
			if exiting || action.Kind == WalkReplaceSkip {
				frame.index = index + len(action.Nodes)
			}
		default:
			phase := "enter"
			if exiting {
				phase = "exit"
			}
			return nodes, fmt.Errorf("Invalid `WalkAction.Unknown(%d)` in %s.", action.Kind, phase)
		}
	}
	return nodes, nil
}
