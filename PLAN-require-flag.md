# Implementation Plan: `--require` flag for `bd mol bond`

## Problem Statement

When a formula is bonded to an existing bead (head), the current behavior creates:
```
spawned_root depends-on head (spawned work is blocked BY head)
```

This structure doesn't surface the workflow steps to the agent. When the agent reads the head bead with `bd show`, they don't see the steps in "Depends on" section - only in "Blocks" section as a single mol-epic.

## Solution

Add `--require` flag that instead creates:
```
head depends-on first-step(s) (head is blocked by first steps)
head depends-on last-step(s) (head is blocked by last steps)
```

When an agent reads the head bead, they immediately see:
```
Depends on (2):
  → gt-xyz1: Sync to main [open]
  → gt-xyz5: Complete development [open]
```

The head cannot be closed until all required steps are complete.

## Design

### Flag Interactions

**`--require` is ONLY valid with proto+mol bonding.** Error for proto+proto or mol+mol.

**`--require` vs `--type`**:
- `--require` REPLACES the default attachment dependency (not in addition to)
- `--type` is ignored when `--require` is specified (with warning if explicitly set)
- No parent-child link to head (avoids transitive blocking)

**Why no parent-child to head**: SQLite ready/blocking logic propagates blockage through parent-child links. If head is blocked by required steps AND spawned tree is child of head, the steps themselves become blocked - defeating the purpose.

### Finding First and Last Steps

After spawning a molecule from a formula, we have:
- `subgraph.Dependencies` - all template dependencies
- `spawnResult.IDMapping` - template ID → spawned ID mapping
- `subgraph.Root.ID` - template root ID

**Blocking dependency types** (all affect ready work):
- `DepBlocks`
- `DepConditionalBlocks`
- `DepWaitsFor`
- Gate-generated issues (via waits_for field in formula)

**First steps** = non-root issues that:
- Are NOT `IssueType=epic` (skip container epics to find actionable nodes)
- Have NO blocking dependencies within the molecule (nothing blocks them)

**Last steps** = non-root issues that:
- Are NOT `IssueType=epic`
- Are NOT the `DependsOnID` for any blocking dependency (nothing depends on them)

Note: This computes over ALL spawned nodes, not just direct children of root. This handles nested formulas correctly.

### Changes Required

1. **Add flag to `molBondCmd`** in `mol_bond.go`:
   ```go
   molBondCmd.Flags().Bool("require", false, "Block head on first/last steps (proto+mol only)")
   ```

2. **Add validation in `runMolBond`**:
   - Error if `--require` used with proto+proto or mol+mol
   - Warn if `--require` used with explicit `--type` flag

3. **Add helper function** `findFirstAndLastSteps`:
   ```go
   func findFirstAndLastSteps(subgraph *TemplateSubgraph) (firstSteps, lastSteps []string, err error)
   ```
   - Returns template IDs of first and last steps (non-epic, non-root)
   - Considers ALL blocking dep types: DepBlocks, DepConditionalBlocks, DepWaitsFor
   - Error if no qualifying first or last steps found

4. **Modify `bondProtoMolWithSubgraph`** to accept `requireFlag bool`:
   - If `requireFlag` is false: current behavior (spawned_root depends-on head)
   - If `requireFlag` is true:
     - Find first/last steps from subgraph
     - Map template IDs to spawned IDs via IDMapping
     - Create `head depends-on step` for each first and last step
     - **Do NOT create** parent-child or blocks link from spawned root to head
     - Deduplicate if first == last (single-step formula)

5. **Update dry-run output**:
   - Dry-run with `--require` must load/cook the proto subgraph
   - Show which steps would become dependencies of head
   - Warn about additional cost vs regular dry-run

### API Changes

```bash
# Current behavior (unchanged)
bd mol bond mol-bug-reproduce gt-abc

# New --require behavior
bd mol bond mol-bug-reproduce gt-abc --require
```

### Dependency Semantics

Current (without --require):
```
Dependency {
    IssueID:     spawned_root,  // mol-epic
    DependsOnID: head,          // target bead
    Type:        blocks,        // spawned_root is blocked BY head
}
```
Agent reads head → sees mol-epic in "Blocks" section, but NOT steps in "Depends on".

New (with --require):
```
Dependency {
    IssueID:     head,        // target bead
    DependsOnID: first-step,  // spawned step
    Type:        blocks,      // head is blocked BY first-step
}
Dependency {
    IssueID:     head,        // target bead
    DependsOnID: last-step,   // spawned step
    Type:        blocks,      // head is blocked BY last-step
}
// NO dependency from spawned_root to head - spawned steps start ready
```
Agent reads head → sees first-step and last-step in "Depends on" section.
Spawned steps are immediately ready (not blocked by head).

### Edge Cases

1. **Multiple first/last steps**: Formulas can have parallel paths. Handle by creating dependencies to ALL first steps and ALL last steps.

2. **Single-step formulas**: First and last are the same issue. Create only one dependency (deduplicate).

3. **Proto (not formula)**: Works the same way - find steps with no blockers/no dependents.

4. **No qualifying steps**: If no first or last steps can be found (all are epics, or empty formula), return error: "formula has no actionable steps for --require mode".

5. **Gate-generated issues**: Steps with `Gate` field in formula generate a separate gate issue that blocks them. These gates ARE actionable first steps (they're the entry point).

6. **Re-bonding (duplicate deps)**: If head already has a dependency to the same step ID, skip silently (don't error on uniqueness constraint).

7. **Nested epics**: Direct children might be container epics. Compute over ALL spawned non-epic nodes to find true first/last.

8. **`--require` with wrong operand types**: Error immediately for proto+proto or mol+mol.

## Testing

1. Bond formula without --require → verify spawned_root depends-on head
2. Bond formula with --require → verify head depends-on first-step, head depends-on last-step
3. Bond multi-step sequential formula → verify correct first/last identification
4. Bond parallel-start formula → verify multiple first deps
5. Bond parallel-end formula → verify multiple last deps
6. Verify dry-run output shows correct structure for both modes
7. **waits_for formula case**: Verify step with waits_for is NOT considered first (the gate IS first)
8. **Gate case**: Verify gate-generated issue IS considered first step
9. **Steps remain ready test**: Verify spawned steps are NOT blocked by head being blocked (catches accidental parent-child attachment)
10. Verify `--require` errors for proto+proto bonding
11. Verify `--require` errors for mol+mol bonding
12. Verify `--require` warns when combined with explicit `--type`
13. Bond single-step formula → verify deduplication (only one dep created)
14. Bond formula with all epics → verify error "no actionable steps"

## Files to Modify

1. `cmd/bd/mol_bond.go` - Add flag, validation, update bond functions
2. `cmd/bd/template.go` - Add `findFirstAndLastSteps` helper (near other subgraph helpers)

## Review Notes (from Codex)

- SQLite ready/blocking logic propagates through parent-child: `internal/storage/sqlite/schema.go`, `internal/storage/sqlite/blocked_cache.go`
- `DependencyType.AffectsReadyWork()` in `internal/types/types.go` - defines which deps block ready work
- Gate issues generated in `cmd/bd/cook.go` via `createGateIssue`
- Existing parallel analysis in `cmd/bd/mol_show.go` handles blocking deps similarly
