// Copyright (c) 2026 TREVARIX Corp.
// SPDX-License-Identifier: AGPL-3.0-or-later

package skills

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// preEditCommand is the hook command every harness runs. It reads the
// harness's JSON payload on stdin and always exits 0, so it can never block or
// fail the agent's edit — important for harnesses where a non-zero exit code
// vetoes the action.
const preEditCommand = "avc hook pre-edit"

// hookCommandFor returns the hook command for framework, naming the harness so
// the checkpoint is attributed to it.
func hookCommandFor(framework string) string {
	return preEditCommand + " --agent " + framework
}

// hookSpec describes where one harness keeps project hooks and how one AVC
// hook entry looks there.
type hookSpec struct {
	rel     string         // project-relative hooks file
	topKeys map[string]any // top-level keys the file format requires
	event   string         // event name under "hooks"
	entry   any            // the entry appended to that event's list
	note    string         // caveat shown to the user, or ""
}

// hookSpecs maps each framework that has a project-level pre-edit hook to its
// spec. Frameworks absent here have no such hook to install.
var hookSpecs = map[string]hookSpec{
	FrameworkClaudeCode: {
		rel:   filepath.Join(".claude", "settings.json"),
		event: "PreToolUse",
		entry: map[string]any{
			"matcher": "Write|Edit",
			"hooks":   []any{map[string]any{"type": "command", "command": preEditCommand}},
		},
	},
	FrameworkWindsurf: {
		rel:   filepath.Join(".windsurf", "hooks.json"),
		event: "pre_write_code",
		entry: map[string]any{"command": hookCommandFor(FrameworkWindsurf), "show_output": false},
	},
	FrameworkCodex: {
		rel:   filepath.Join(".codex", "hooks.json"),
		event: "PreToolUse",
		entry: map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": hookCommandFor(FrameworkCodex)}},
		},
		note: "Codex loads project hooks only for trusted projects, and its PreToolUse event may fire only for shell commands — the checkpoint is then taken before the session's first command rather than its first edit.",
	},
	FrameworkCopilot: {
		rel:     filepath.Join(".github", "hooks", "avc.json"),
		topKeys: map[string]any{"version": 1},
		event:   "PreToolUse",
		entry:   map[string]any{"type": "command", "command": hookCommandFor(FrameworkCopilot)},
		note:    "Copilot agent hooks are in preview in VS Code; check that hooks are enabled in your build.",
	},
}

// writeHook installs the AVC pre-edit hook for framework, if it has one,
// merging into an existing hooks file without disturbing other hooks.
func writeHook(projectRoot, framework string, r *WriteResult) error {
	spec, ok := hookSpecs[framework]
	if !ok {
		r.Warnings = append(r.Warnings, framework+" has no project-level pre-edit hook; --hooks was ignored for it")
		return nil
	}
	path := filepath.Join(projectRoot, spec.rel)
	root, existed, err := readJSONObject(path)
	if err != nil {
		return err
	}

	for k, v := range spec.topKeys {
		if _, set := root[k]; !set {
			root[k] = v
		}
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		root["hooks"] = hooks
	}
	list, _ := hooks[spec.event].([]any)
	if containsPreEdit(list) {
		r.Actions = append(r.Actions, fileAction(filepath.ToSlash(spec.rel), skipAlreadyConfigured))
		return nil
	}
	hooks[spec.event] = append(list, spec.entry)

	if err := writeJSON(path, root); err != nil {
		return err
	}
	action := actionCreated
	if existed {
		action = actionUpdated
	}
	r.Actions = append(r.Actions, fileAction(filepath.ToSlash(spec.rel), action))
	if spec.note != "" {
		r.Warnings = append(r.Warnings, spec.note)
	}
	return nil
}

// containsPreEdit reports whether any hook entry already runs the AVC command.
func containsPreEdit(list []any) bool {
	for _, e := range list {
		data, err := json.Marshal(e)
		if err == nil && strings.Contains(string(data), preEditCommand) {
			return true
		}
	}
	return false
}
