// Package preset holds NabuGate's two kinds of ready-made prompt bundles:
//
//   - Skills are reusable instructions a caller switches on per request
//     ("nabu_skills": ["json-extract"] or the X-Nabu-Skills header). A skill
//     adds a system instruction and, optionally, a response_format and a
//     default temperature. It never picks the model: it rides whatever model
//     or agent the caller asked for.
//   - Templates are complete tasks — Persian copywriting, support replies,
//     translation, code review, SEO, Instagram captions — with named
//     variables. POST /v1/templates/{name}/run fills the variables into the
//     template's prompt and runs it as an ordinary chat completion, so it is
//     billed, rate-limited and allow-listed exactly like one.
//
// Both are plain YAML files loaded from a directory at start-up, the same way
// agents and flows are, so adding one is a file, not a code change.
package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Text is a string in Persian and English. Either may be empty; Get falls back
// to the other so a half-translated preset still shows something.
type Text struct {
	FA string `yaml:"fa" json:"fa,omitempty"`
	EN string `yaml:"en" json:"en,omitempty"`
}

// Get returns the text in lang ("fa" or anything else for English).
func (t Text) Get(lang string) string {
	if lang == "fa" && t.FA != "" {
		return t.FA
	}
	if t.EN != "" {
		return t.EN
	}
	return t.FA
}

// Skill is one reusable instruction bundle.
type Skill struct {
	Name        string   `yaml:"name" json:"name"`
	Title       Text     `yaml:"title" json:"title"`
	Description Text     `yaml:"description" json:"description"`
	Category    string   `yaml:"category" json:"category,omitempty"`
	System      string   `yaml:"system" json:"system"`
	Temperature *float64 `yaml:"temperature" json:"temperature,omitempty"`
	// ResponseFormat is "json_object" or empty. Only applied when the caller
	// did not send its own response_format.
	ResponseFormat string `yaml:"response_format" json:"response_format,omitempty"`
}

// Variable is one blank a template's prompt fills in.
type Variable struct {
	Name        string `yaml:"name" json:"name"`
	Label       Text   `yaml:"label" json:"label"`
	Placeholder Text   `yaml:"placeholder" json:"placeholder,omitempty"`
	Required    bool   `yaml:"required" json:"required,omitempty"`
	Multiline   bool   `yaml:"multiline" json:"multiline,omitempty"`
	Default     string `yaml:"default" json:"default,omitempty"`
	// Options, when set, limits the value to one of these (a select box).
	Options []string `yaml:"options" json:"options,omitempty"`
}

// Template is one ready task.
type Template struct {
	Name        string     `yaml:"name" json:"name"`
	Title       Text       `yaml:"title" json:"title"`
	Description Text       `yaml:"description" json:"description"`
	Category    string     `yaml:"category" json:"category,omitempty"`
	Model       string     `yaml:"model" json:"model"`
	System      string     `yaml:"system" json:"system,omitempty"`
	Prompt      string     `yaml:"prompt" json:"prompt"`
	Variables   []Variable `yaml:"variables" json:"variables"`
	Skills      []string   `yaml:"skills" json:"skills,omitempty"`
	Temperature *float64   `yaml:"temperature" json:"temperature,omitempty"`
	MaxTokens   *int       `yaml:"max_tokens" json:"max_tokens,omitempty"`
}

// MaxSkillsPerRequest bounds how many skills one request may stack, so a
// caller cannot turn one call into a wall of system prompt.
const MaxSkillsPerRequest = 4

// MaxVariableBytes bounds one filled-in variable.
const MaxVariableBytes = 64 << 10

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var varRE = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// Registry holds every loaded skill and template.
type Registry struct {
	skills    map[string]Skill
	templates map[string]Template
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{skills: map[string]Skill{}, templates: map[string]Template{}}
}

// AddSkill validates and registers a skill.
func (r *Registry) AddSkill(s Skill) error {
	s.Name = strings.TrimSpace(s.Name)
	if !nameRE.MatchString(s.Name) {
		return fmt.Errorf("skill name %q must be lower-case letters, digits, '.', '_' or '-'", s.Name)
	}
	if strings.TrimSpace(s.System) == "" {
		return fmt.Errorf("skill %q has no system instruction", s.Name)
	}
	switch s.ResponseFormat {
	case "", "json_object":
	default:
		return fmt.Errorf("skill %q: response_format must be empty or json_object", s.Name)
	}
	if _, dup := r.skills[s.Name]; dup {
		return fmt.Errorf("duplicate skill %q", s.Name)
	}
	r.skills[s.Name] = s
	return nil
}

// AddTemplate validates and registers a template. Every {{var}} in the prompt
// must be declared, and every skill it names must already be registered —
// load skills first.
func (r *Registry) AddTemplate(t Template) error {
	t.Name = strings.TrimSpace(t.Name)
	if !nameRE.MatchString(t.Name) {
		return fmt.Errorf("template name %q must be lower-case letters, digits, '.', '_' or '-'", t.Name)
	}
	if strings.TrimSpace(t.Model) == "" {
		return fmt.Errorf("template %q has no model", t.Name)
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return fmt.Errorf("template %q has no prompt", t.Name)
	}
	declared := map[string]bool{}
	for _, v := range t.Variables {
		if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(v.Name) {
			return fmt.Errorf("template %q: bad variable name %q", t.Name, v.Name)
		}
		if declared[v.Name] {
			return fmt.Errorf("template %q: variable %q declared twice", t.Name, v.Name)
		}
		declared[v.Name] = true
	}
	for _, m := range varRE.FindAllStringSubmatch(t.Prompt+t.System, -1) {
		if !declared[m[1]] {
			return fmt.Errorf("template %q uses {{%s}} but does not declare it", t.Name, m[1])
		}
	}
	for _, sk := range t.Skills {
		if _, ok := r.skills[sk]; !ok {
			return fmt.Errorf("template %q names unknown skill %q", t.Name, sk)
		}
	}
	if _, dup := r.templates[t.Name]; dup {
		return fmt.Errorf("duplicate template %q", t.Name)
	}
	r.templates[t.Name] = t
	return nil
}

// Skill returns a skill by name. Safe on a nil registry.
func (r *Registry) Skill(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	s, ok := r.skills[name]
	return s, ok
}

// Template returns a template by name. Safe on a nil registry.
func (r *Registry) Template(name string) (Template, bool) {
	if r == nil {
		return Template{}, false
	}
	t, ok := r.templates[name]
	return t, ok
}

// Skills lists every skill sorted by name.
func (r *Registry) Skills() []Skill {
	if r == nil {
		return nil
	}
	out := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Templates lists every template sorted by category, then name.
func (r *Registry) Templates() []Template {
	if r == nil {
		return nil
	}
	out := make([]Template, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ResolveSkills turns caller-supplied names into skills, de-duplicated in the
// order given. An unknown name or too many names is an error the caller sees,
// rather than a skill silently not applied.
func (r *Registry) ResolveSkills(names []string) ([]Skill, error) {
	seen := map[string]bool{}
	var out []Skill
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		s, ok := r.Skill(n)
		if !ok {
			return nil, fmt.Errorf("unknown skill %q (GET /v1/skills lists them)", n)
		}
		out = append(out, s)
	}
	if len(out) > MaxSkillsPerRequest {
		return nil, fmt.Errorf("at most %d skills per request", MaxSkillsPerRequest)
	}
	return out, nil
}

// SystemText joins the skills' instructions into one system message.
func SystemText(skills []Skill) string {
	parts := make([]string, 0, len(skills))
	for _, s := range skills {
		parts = append(parts, strings.TrimSpace(s.System))
	}
	return strings.Join(parts, "\n\n")
}

// Render fills the template's variables. A missing required variable, an
// unknown variable, a value outside Options, or an oversized value is an error.
// It returns the system and user text to send.
func (t Template) Render(vars map[string]string) (system, user string, err error) {
	known := map[string]Variable{}
	for _, v := range t.Variables {
		known[v.Name] = v
	}
	for k := range vars {
		if _, ok := known[k]; !ok {
			return "", "", fmt.Errorf("template %q has no variable %q", t.Name, k)
		}
	}
	values := map[string]string{}
	for _, v := range t.Variables {
		val := strings.TrimSpace(vars[v.Name])
		if val == "" {
			val = v.Default
		}
		if val == "" && v.Required {
			return "", "", fmt.Errorf("variable %q is required", v.Name)
		}
		if len(val) > MaxVariableBytes {
			return "", "", fmt.Errorf("variable %q is longer than %d bytes", v.Name, MaxVariableBytes)
		}
		if len(v.Options) > 0 && val != "" {
			ok := false
			for _, o := range v.Options {
				if o == val {
					ok = true
					break
				}
			}
			if !ok {
				return "", "", fmt.Errorf("variable %q must be one of %s", v.Name, strings.Join(v.Options, ", "))
			}
		}
		values[v.Name] = val
	}
	fill := func(s string) string {
		return varRE.ReplaceAllStringFunc(s, func(m string) string {
			return values[varRE.FindStringSubmatch(m)[1]]
		})
	}
	return strings.TrimSpace(fill(t.System)), strings.TrimSpace(fill(t.Prompt)), nil
}

// ParseSkillNames reads a request's skill list from the body field (a JSON
// array of strings, or one comma-separated string) and the header value.
func ParseSkillNames(body json.RawMessage, header string) ([]string, error) {
	var names []string
	if len(body) > 0 && string(body) != "null" {
		var list []string
		if err := json.Unmarshal(body, &list); err != nil {
			var one string
			if err2 := json.Unmarshal(body, &one); err2 != nil {
				return nil, fmt.Errorf("nabu_skills must be an array of skill names")
			}
			list = strings.Split(one, ",")
		}
		names = append(names, list...)
	}
	if header = strings.TrimSpace(header); header != "" {
		names = append(names, strings.Split(header, ",")...)
	}
	return names, nil
}

// Load reads every *.yaml/*.yml in skillsDir and templatesDir (either may be
// empty). Bad files are skipped with a warning, like agents: one broken preset
// must not take the gateway down.
func Load(skillsDir, templatesDir string) (*Registry, []string) {
	reg := NewRegistry()
	var warnings []string
	for _, path := range yamlFiles(skillsDir, &warnings) {
		var s Skill
		if err := readYAML(path, &s); err != nil {
			warnings = append(warnings, fmt.Sprintf("skip skill file %q: %v", path, err))
			continue
		}
		if s.Name == "" {
			s.Name = baseName(path)
		}
		if err := reg.AddSkill(s); err != nil {
			warnings = append(warnings, fmt.Sprintf("skip skill file %q: %v", path, err))
		}
	}
	for _, path := range yamlFiles(templatesDir, &warnings) {
		var t Template
		if err := readYAML(path, &t); err != nil {
			warnings = append(warnings, fmt.Sprintf("skip template file %q: %v", path, err))
			continue
		}
		if t.Name == "" {
			t.Name = baseName(path)
		}
		if err := reg.AddTemplate(t); err != nil {
			warnings = append(warnings, fmt.Sprintf("skip template file %q: %v", path, err))
		}
	}
	return reg, warnings
}

func yamlFiles(dir string, warnings *[]string) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("preset dir %q: %v", dir, err))
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(e.Name())); ext == ".yaml" || ext == ".yml" {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func readYAML(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, v)
}

func baseName(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}
