// Package skills embeds the agent skills so `yuque-cli skill install` works from a single binary.
package skills

import "embed"

//go:embed yuque-reader/SKILL.md
var FS embed.FS

// Name is both the skill's directory name and its frontmatter name.
const Name = "yuque-reader"
