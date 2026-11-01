package fi

import (
	"context"
	"fmt"
	"strings"

	"github.com/Shougo/goda.vim/sdk"
)

// The source and the kind "action", of every configuration: the item
// actions of items of another session, to choose one (the UI action
// chooseAction, as ddu.vim's source action).

// actionSource gathers the actions in its params: {name, ids, actions:
// [{name, description}]}.
type actionSource struct{}

type actionParams struct {
	// Name of the session of the items; IDs of the items there.
	Name    string        `json:"name"`
	IDs     []uint64      `json:"ids"`
	Actions []actionEntry `json:"actions"`
}

type actionEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ActionTarget is the action of an item of the kind "action".
type ActionTarget struct {
	Name   string   `json:"name"`
	Action string   `json:"action"`
	IDs    []uint64 `json:"ids"`
}

func (actionSource) Name() string { return "action" }
func (actionSource) Kind() string { return "action" }

func (actionSource) Gather(_ context.Context, _ sdk.Host, a GatherArgs, emit func([]Item) error) error {
	var p actionParams
	if err := Decode(a.SourceParams, &p); err != nil {
		return err
	}
	width := 0
	for _, e := range p.Actions {
		width = max(width, StrWidth(e.Name))
	}
	items := make([]Item, 0, len(p.Actions))
	for _, e := range p.Actions {
		it := Item{Word: e.Name, Action: ActionTarget{Name: p.Name, Action: e.Name, IDs: p.IDs}}
		if e.Description != "" {
			it.Display = e.Name + strings.Repeat(" ", width-StrWidth(e.Name)+1) + e.Description
			it.Highlights = []Highlight{{Name: "action-description", HlGroup: "Comment", Col: len(e.Name) + width - StrWidth(e.Name) + 2, Width: len(e.Description)}}
		}
		items = append(items, it)
	}
	return emit(items)
}

// actionKind runs the action chosen: "do", its default, goes back to the
// session of the items (goda_fi#pop()) and runs it there.
type actionKind struct{}

func (actionKind) Name() string { return "action" }

func (actionKind) Actions() map[string]Action {
	return map[string]Action{
		"do": {
			Description: "Run the action on the items of the session it was chosen for.",
			Run: func(ctx context.Context, h sdk.Host, a ActionArgs) (ActionResult, error) {
				if len(a.Items) == 0 {
					return ActionResult{}, nil
				}
				var t ActionTarget
				if err := sdk.Decode(a.Items[0].Action, &t); err != nil {
					return ActionResult{}, err
				}
				// Once this action is done: the session goes away.
				_, err := h.Call(ctx, "goda_fi#ui#_do_chosen", t.Name, t.Action, t.IDs)
				return ActionResult{}, err
			},
		},
	}
}

// chooseOptions are those of the session that chooses an action of items
// of the session st: pushed on its name, with the source action.
func (st *state) chooseOptions(items []Item) (map[string]any, error) {
	set, err := st.actionsOf(items)
	if err != nil {
		return nil, err
	}
	names := set.names(st.opts.Actions)
	entries := make([]map[string]any, 0, len(names))
	for _, n := range names {
		desc := ""
		actual := n
		if to, ok := set.aliases[n]; ok {
			actual = to
		}
		if a, ok := set.actions[actual]; ok {
			desc = a.Description
			if actual != n {
				desc = fmt.Sprintf("(%s) %s", actual, desc)
			}
		}
		entries = append(entries, map[string]any{"name": n, "description": desc})
	}
	ids := make([]uint64, len(items))
	for i, it := range items {
		ids[i] = it.id
	}
	ui := st.opts.UI
	if _, ok := st.e.b.uis["ff"]; ok {
		ui = "ff"
	}
	return map[string]any{
		"name": st.name, "push": true, "ui": ui,
		"sources": []any{map[string]any{
			"name":   "action",
			"params": map[string]any{"name": st.name, "ids": ids, "actions": entries},
		}},
		"sourceOptions": map[string]any{"action": map[string]any{
			"defaultAction": "do", "columns": []any{}, "path": st.ctx.Path,
		}},
	}, nil
}

// ChooseAction is what a UI action chooseAction does: a session pushed on
// this one that lists the actions of items, and runs the one chosen.
func (a sessionAPI) ChooseAction(ctx context.Context, items []Item) error {
	o, err := a.st.chooseOptions(items)
	if err != nil {
		return err
	}
	// The start puts this session aside without waiting for it.
	_, err = a.st.h.Call(ctx, "goda_fi#start", o)
	return err
}

func (a sessionAPI) ItemActionDescriptions(items []Item) map[string]string {
	set, err := a.st.actionsOf(items)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, n := range set.names(a.st.opts.Actions) {
		actual := n
		if to, ok := set.aliases[n]; ok {
			actual = to
		}
		out[n] = set.actions[actual].Description
	}
	return out
}
