package config

import (
	"fmt"
	"testing"
)

// The Mr ChatGPT story agent ships with per-network references; a YAML slip
// would silently drop them, so the checked-in file is loaded here.
func TestStoryAgentFileLoadsWithReferences(t *testing.T) {
	for i := 1; i <= 6; i++ {
		t.Setenv(fmt.Sprintf("MRCHATGPT_REF_%d", i), fmt.Sprintf("https://img.example/%d.jpg", i))
	}
	ac, err := loadAgentFile("../../agents/mrchatgpt-story.yaml")
	if err != nil {
		t.Fatal(err)
	}
	a := ac.toAgent("mrchatgpt-story")
	if a.Model != "nabu-vision" || len(a.ReferencesFor("instagram")) != 6 || len(a.ReferencesFor("telegram")) != 3 {
		t.Fatalf("agent = model %q, instagram %d, telegram %d",
			a.Model, len(a.ReferencesFor("instagram")), len(a.ReferencesFor("telegram")))
	}
}
