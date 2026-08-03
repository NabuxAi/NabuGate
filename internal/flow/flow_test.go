package flow

import "testing"

func TestRenderDefaultsToThePreviousOutput(t *testing.T) {
	// The ordinary chain — "hand step two what step one said" — must need no
	// template at all, or every flow anyone writes carries the same boilerplate.
	got := Step{Agent: "reviewer"}.Render("the draft", "the brief")
	if got != "the draft" {
		t.Errorf("Render() = %q, want the draft", got)
	}
}

func TestRenderFillsBothPlaceholders(t *testing.T) {
	step := Step{Agent: "reviewer", Input: "BRIEF: {{input}}\nDRAFT: {{previous}}"}

	got := step.Render("the draft", "the brief")
	want := "BRIEF: the brief\nDRAFT: the draft"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestDisplayNameFallsBackToTheAgent(t *testing.T) {
	if got := (Step{Agent: "reviewer"}).DisplayName(); got != "reviewer" {
		t.Errorf("DisplayName() = %q", got)
	}
	if got := (Step{Agent: "reviewer", Label: "second opinion"}).DisplayName(); got != "second opinion" {
		t.Errorf("DisplayName() = %q", got)
	}
}

func TestAddRefusesAFlowThatCannotRun(t *testing.T) {
	reg := NewRegistry()

	cases := map[string]Flow{
		"no name":  {Steps: []Step{{Agent: "a"}}},
		"no steps": {Name: "empty"},
		"blank step": {Name: "gap", Steps: []Step{
			{Agent: "a"}, {Agent: "  "},
		}},
	}

	for name, f := range cases {
		if err := reg.Add(f); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	if reg.Len() != 0 {
		t.Errorf("a refused flow must not be registered, have %d", reg.Len())
	}
}

func TestAddRefusesADuplicateAndSetOverwrites(t *testing.T) {
	reg := NewRegistry()
	first := Flow{Name: "team", Steps: []Step{{Agent: "a"}}}

	if err := reg.Add(first); err != nil {
		t.Fatal(err)
	}

	// A second definition must not silently shadow the first — which of them
	// ran would then depend on load order nobody can see.
	if err := reg.Add(Flow{Name: "team", Steps: []Step{{Agent: "b"}}}); err == nil {
		t.Error("a duplicate name should be refused")
	}

	// Set is the console's path: editing a flow at runtime replaces it.
	if err := reg.Set(Flow{Name: "team", Steps: []Step{{Agent: "b"}}}); err != nil {
		t.Fatal(err)
	}
	got, _ := reg.Lookup("team")
	if got.Steps[0].Agent != "b" {
		t.Errorf("Set did not overwrite, first step is %q", got.Steps[0].Agent)
	}
}

func TestNilRegistryLooksUpNothingRatherThanPanicking(t *testing.T) {
	// Every deployment without flows takes this path on every request.
	var reg *Registry

	if _, ok := reg.Lookup("anything"); ok {
		t.Error("a nil registry should find nothing")
	}
	if reg.Len() != 0 || reg.Names() != nil {
		t.Error("a nil registry should be empty")
	}
	reg.Remove("anything")
}
