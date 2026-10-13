package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type SkillFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseSkill(filepath string) (*SkillFrontMatter, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(string(content), "---\n", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("expected at least 3 parts, got %d", len(parts))
	}

	var result SkillFrontMatter
	if err := yaml.Unmarshal([]byte(parts[1]), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func skillInstructions() (string, error) {
	var skills []*SkillFrontMatter

	err := filepath.WalkDir(".claude/skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}

		skill, parseError := parseSkill(path)
		if parseError != nil {
			return parseError
		}

		skills = append(skills, skill)
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	builder := strings.Builder{}
	builder.WriteString("You have access to the following skills:\n\n")
	for _, skill := range skills {
		builder.WriteString(fmt.Sprintf("%s: %s\n", skill.Name, skill.Description))
	}
	return builder.String(), nil
}
