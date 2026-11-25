package preset

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestShippedPresetsLoad loads the repo's presets/ directory exactly as the
// image does, so a broken YAML file or a template naming a missing skill fails
// here instead of being skipped with a warning in production.
func TestShippedPresetsLoad(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "presets")
	reg, warnings := Load(filepath.Join(root, "skills"), filepath.Join(root, "templates"))
	for _, w := range warnings {
		t.Error(w)
	}
	if len(reg.Skills()) < 8 || len(reg.Templates()) < 10 {
		t.Fatalf("loaded %d skills, %d templates", len(reg.Skills()), len(reg.Templates()))
	}
	for _, s := range reg.Skills() {
		if s.Title.FA == "" || s.Title.EN == "" || s.Description.FA == "" || s.Description.EN == "" {
			t.Errorf("skill %q needs Persian and English title and description", s.Name)
		}
	}
	for _, tp := range reg.Templates() {
		if tp.Title.FA == "" || tp.Title.EN == "" {
			t.Errorf("template %q needs Persian and English titles", tp.Name)
		}
		vars := map[string]string{}
		for _, v := range tp.Variables {
			if v.Label.FA == "" || v.Label.EN == "" {
				t.Errorf("template %q variable %q needs both labels", tp.Name, v.Name)
			}
			if v.Required {
				vars[v.Name] = "x"
				if len(v.Options) > 0 {
					vars[v.Name] = v.Options[0]
				}
			}
		}
		if _, _, err := tp.Render(vars); err != nil {
			t.Errorf("template %q does not render with its required variables: %v", tp.Name, err)
		}
	}
}

func TestTemplateValidation(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddTemplate(Template{Name: "a", Model: "m", Prompt: "{{x}}"}); err == nil {
		t.Error("undeclared variable accepted")
	}
	if err := reg.AddTemplate(Template{Name: "b", Model: "m", Prompt: "p", Skills: []string{"nope"}}); err == nil {
		t.Error("unknown skill accepted")
	}
	if err := reg.AddTemplate(Template{Name: "Bad Name", Model: "m", Prompt: "p"}); err == nil {
		t.Error("bad name accepted")
	}
}

func TestResolveSkillsLimits(t *testing.T) {
	reg := NewRegistry()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		if err := reg.AddSkill(Skill{Name: n, System: n}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reg.ResolveSkills([]string{"a", "b", "c", "d", "e"}); err == nil {
		t.Error("more than MaxSkillsPerRequest accepted")
	}
	got, err := reg.ResolveSkills([]string{"a", " a", "b"})
	if err != nil || len(got) != 2 {
		t.Errorf("dedupe: %v %v", got, err)
	}
	names, err := ParseSkillNames([]byte(`"a, b"`), "c")
	if err != nil || len(names) != 3 {
		t.Errorf("ParseSkillNames = %v %v", names, err)
	}
}

func TestRenderDefaults(t *testing.T) {
	tp := Template{Name: "t", Model: "m", Prompt: "{{a}}-{{b}}", Variables: []Variable{{Name: "a", Required: true}, {Name: "b", Default: "d"}}}
	_, u, err := tp.Render(map[string]string{"a": "x"})
	if err != nil || u != "x-d" {
		t.Errorf("Render = %q %v", u, err)
	}
}
