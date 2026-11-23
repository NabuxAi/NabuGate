package agent

import "testing"

func TestRegistryAddAndLookup(t *testing.T) {
	r := NewRegistry()
	temp := 0.5
	if err := r.Add(Agent{Name: "director", Model: "nabu-smart", System: "You direct.", Temperature: &temp}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, ok := r.Lookup("director")
	if !ok {
		t.Fatal("director not found")
	}
	if got.Model != "nabu-smart" || got.System != "You direct." {
		t.Errorf("agent = %+v", got)
	}
	if got.Temperature == nil || *got.Temperature != 0.5 {
		t.Errorf("temperature not preserved: %+v", got.Temperature)
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Error("unexpected agent 'missing'")
	}
}

func TestRegistryValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.Add(Agent{Name: "", Model: "nabu-smart"}); err == nil {
		t.Error("empty name should be rejected")
	}
	if err := r.Add(Agent{Name: "x", Model: ""}); err == nil {
		t.Error("missing model should be rejected")
	}
	if err := r.Add(Agent{Name: "  writer ", Model: "nabu-fast"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Name is trimmed on store, so the trimmed form is what resolves.
	if _, ok := r.Lookup("writer"); !ok {
		t.Error("trimmed name should be looked up as 'writer'")
	}
	if err := r.Add(Agent{Name: "writer", Model: "nabu-smart"}); err == nil {
		t.Error("duplicate name should be rejected")
	}
}

func TestRegistryNamesSortedAndNilSafe(t *testing.T) {
	var nilReg *Registry
	if nilReg.Len() != 0 || nilReg.Names() != nil {
		t.Error("nil registry should be empty")
	}
	if _, ok := nilReg.Lookup("anything"); ok {
		t.Error("nil registry Lookup should be not-found")
	}

	r := NewRegistry()
	for _, n := range []string{"gamma", "alpha", "beta"} {
		if err := r.Add(Agent{Name: n, Model: "nabu-smart"}); err != nil {
			t.Fatal(err)
		}
	}
	names := r.Names()
	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("names = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
	if r.Len() != 3 {
		t.Errorf("Len = %d, want 3", r.Len())
	}
}

func TestReferencesForFallsBackAndCaps(t *testing.T) {
	many := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		many = append(many, "https://img.example/"+string(rune('a'+i))+".jpg")
	}
	a := Agent{References: map[string][]string{"instagram": many, "default": {" https://img.example/d.jpg "}}}
	if got := a.ReferencesFor(" INSTAGRAM "); len(got) != MaxReferenceImages {
		t.Errorf("instagram refs = %d, want capped at %d", len(got), MaxReferenceImages)
	}
	if got := a.ReferencesFor("linkedin"); len(got) != 1 || got[0] != "https://img.example/d.jpg" {
		t.Errorf("linkedin refs = %v, want the trimmed default", got)
	}
	if got := (Agent{}).ReferencesFor("instagram"); got != nil {
		t.Errorf("no references = %v, want nil", got)
	}
}
