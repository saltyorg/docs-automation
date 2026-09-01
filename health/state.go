package health

import "slices"

// StateVersion is the version of the semantic state embedded by consumers.
const StateVersion = 1

// StateFinding is the identity and display label retained for change detection.
type StateFinding struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Label string `json:"label"`
}

// StateResult is the semantic state of one health check.
type StateResult struct {
	Kind       Kind           `json:"kind"`
	Enabled    bool           `json:"enabled"`
	Exemptions int            `json:"exemptions"`
	Findings   []StateFinding `json:"findings"`
}

// State is the canonical, run-independent health state.
type State struct {
	Version int           `json:"version"`
	Results []StateResult `json:"results"`
}

// Changes describes semantic differences between two states.
type Changes struct {
	Added          []StateFinding `json:"added"`
	Resolved       []StateFinding `json:"resolved"`
	ChangedResults []Kind         `json:"changed_results"`
}

// State derives semantic state from canonical report results. RunInfo and
// finding details are intentionally not represented here.
func (r Report) State() State {
	canonical := r.Canonical()
	state := State{Version: StateVersion, Results: make([]StateResult, len(canonical.Results))}
	for i, result := range canonical.Results {
		stateResult := StateResult{
			Kind:       result.Kind,
			Enabled:    result.Enabled,
			Exemptions: result.Exemptions,
			Findings:   make([]StateFinding, 0, len(result.Findings)),
		}
		for _, finding := range result.Findings {
			kind := finding.Kind
			if kind == "" {
				kind = result.Kind
			}
			stateResult.Findings = append(stateResult.Findings, StateFinding{
				ID:    finding.ID(),
				Kind:  kind,
				Label: finding.Label(),
			})
		}
		slices.SortFunc(stateResult.Findings, compareStateFindings)
		state.Results[i] = stateResult
	}
	return state
}

// Diff compares semantic finding identities and result metadata.
func Diff(oldState, newState State) Changes {
	changes := Changes{
		Added:          make([]StateFinding, 0),
		Resolved:       make([]StateFinding, 0),
		ChangedResults: make([]Kind, 0),
	}
	oldFindings := stateFindingMap(oldState)
	newFindings := stateFindingMap(newState)
	for id, finding := range newFindings {
		if _, exists := oldFindings[id]; !exists {
			changes.Added = append(changes.Added, finding)
		}
	}
	for id, finding := range oldFindings {
		if _, exists := newFindings[id]; !exists {
			changes.Resolved = append(changes.Resolved, finding)
		}
	}

	oldResults := stateResultMap(oldState)
	newResults := stateResultMap(newState)
	for kind, result := range newResults {
		oldResult, exists := oldResults[kind]
		if !exists || result.Enabled != oldResult.Enabled || result.Exemptions != oldResult.Exemptions || !sameFindingIDs(oldResult.Findings, result.Findings) {
			changes.ChangedResults = append(changes.ChangedResults, kind)
		}
	}
	for kind := range oldResults {
		if _, exists := newResults[kind]; !exists {
			changes.ChangedResults = append(changes.ChangedResults, kind)
		}
	}

	slices.SortFunc(changes.Added, compareStateFindings)
	slices.SortFunc(changes.Resolved, compareStateFindings)
	slices.SortFunc(changes.ChangedResults, compareKinds)
	return changes
}

func stateFindingMap(state State) map[string]StateFinding {
	findings := make(map[string]StateFinding)
	for _, result := range state.Results {
		for _, finding := range result.Findings {
			findings[finding.ID] = finding
		}
	}
	return findings
}

func stateResultMap(state State) map[Kind]StateResult {
	results := make(map[Kind]StateResult, len(state.Results))
	for _, result := range state.Results {
		results[result.Kind] = result
	}
	return results
}

func sameFindingIDs(oldFindings, newFindings []StateFinding) bool {
	if len(oldFindings) != len(newFindings) {
		return false
	}
	oldIDs := make(map[string]struct{}, len(oldFindings))
	for _, finding := range oldFindings {
		oldIDs[finding.ID] = struct{}{}
	}
	for _, finding := range newFindings {
		if _, exists := oldIDs[finding.ID]; !exists {
			return false
		}
	}
	return true
}

func compareStateFindings(a, b StateFinding) int {
	if comparison := compareKinds(a.Kind, b.Kind); comparison != 0 {
		return comparison
	}
	if a.Label < b.Label {
		return -1
	}
	if a.Label > b.Label {
		return 1
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}
