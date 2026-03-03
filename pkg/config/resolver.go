package config

import (
	"fmt"
	"strings"
)

// Resolve looks up an identifier in the resource map and returns the matching
// entity with its level and full path. The identifier can be:
//   - A bare name (e.g., "aurora") which searches all orgs, pillars, and teams
//   - A slash-delimited path (e.g., "rosa/aurora" or "hcm/rosa/aurora")
//
// If exactly one match is found, it is returned. If multiple matches exist,
// an error with disambiguation instructions is returned. If no match is found,
// a not-found error is returned.
func (rm *ResourceMap) Resolve(identifier string) (*ResolvedEntity, error) {
	parts := strings.Split(identifier, "/")

	switch len(parts) {
	case 1:
		return rm.resolveBare(parts[0])
	case 2:
		return rm.resolveTwoPart(parts[0], parts[1])
	case 3:
		return rm.resolveThreePart(parts[0], parts[1], parts[2])
	default:
		return nil, fmt.Errorf("invalid identifier %q: expected at most 3 segments (org/pillar/team)", identifier)
	}
}

func (rm *ResourceMap) resolveBare(name string) (*ResolvedEntity, error) {
	var matches []ResolvedEntity

	for _, org := range rm.Organizations {
		if org.Identifier == name {
			matches = append(matches, ResolvedEntity{
				Level:        LevelOrganization,
				Organization: org,
				Path:         org.Identifier,
			})
		}
		for _, pillar := range org.Pillars {
			if pillar.Identifier == name {
				matches = append(matches, ResolvedEntity{
					Level:        LevelPillar,
					Organization: org,
					Pillar:       pillar,
					Path:         org.Identifier + "/" + pillar.Identifier,
				})
			}
			for _, team := range pillar.Teams {
				if team.Identifier == name {
					matches = append(matches, ResolvedEntity{
						Level:        LevelTeam,
						Organization: org,
						Pillar:       pillar,
						Team:         team,
						Path:         org.Identifier + "/" + pillar.Identifier + "/" + team.Identifier,
					})
				}
			}
		}
	}

	return selectMatch(name, matches)
}

func (rm *ResourceMap) resolveTwoPart(first, second string) (*ResolvedEntity, error) {
	var matches []ResolvedEntity

	for _, org := range rm.Organizations {
		for _, pillar := range org.Pillars {
			// Interpret as pillar/team
			if pillar.Identifier == first {
				for _, team := range pillar.Teams {
					if team.Identifier == second {
						matches = append(matches, ResolvedEntity{
							Level:        LevelTeam,
							Organization: org,
							Pillar:       pillar,
							Team:         team,
							Path:         org.Identifier + "/" + pillar.Identifier + "/" + team.Identifier,
						})
					}
				}
			}
			// Interpret as org/pillar
			if org.Identifier == first && pillar.Identifier == second {
				matches = append(matches, ResolvedEntity{
					Level:        LevelPillar,
					Organization: org,
					Pillar:       pillar,
					Path:         org.Identifier + "/" + pillar.Identifier,
				})
			}
		}
	}

	return selectMatch(first+"/"+second, matches)
}

func (rm *ResourceMap) resolveThreePart(orgID, pillarID, teamID string) (*ResolvedEntity, error) {
	for _, org := range rm.Organizations {
		if org.Identifier != orgID {
			continue
		}
		for _, pillar := range org.Pillars {
			if pillar.Identifier != pillarID {
				continue
			}
			for _, team := range pillar.Teams {
				if team.Identifier == teamID {
					return &ResolvedEntity{
						Level:        LevelTeam,
						Organization: org,
						Pillar:       pillar,
						Team:         team,
						Path:         orgID + "/" + pillarID + "/" + teamID,
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("identifier %q not found", orgID+"/"+pillarID+"/"+teamID)
}

func selectMatch(identifier string, matches []ResolvedEntity) (*ResolvedEntity, error) {
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("identifier %q not found", identifier)
	case 1:
		return &matches[0], nil
	default:
		var paths []string
		for _, m := range matches {
			paths = append(paths, m.Path)
		}
		return nil, fmt.Errorf("identifier %q is ambiguous, matches: %s. Use a slash-delimited path to disambiguate (e.g., pillar/team or org/pillar/team)",
			identifier, strings.Join(paths, ", "))
	}
}
