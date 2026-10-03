// Package assets contains the offline public contracts and portable skill.
package assets

import "embed"

//go:embed schemas/*.schema.json skills/teamcity-axi/SKILL.md
var Files embed.FS
