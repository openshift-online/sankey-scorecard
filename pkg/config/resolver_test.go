package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
)

func makeResourceMapForResolver() *config.ResourceMap {
	return &config.ResourceMap{
		SprintReferenceDate: "2026-02-11",
		SprintDurationDays:  21,
		Organizations: []config.Organization{
			{
				Name:       "Hybrid Cloud Management",
				Identifier: "hcm",
				Pillars: []config.Pillar{
					{
						Name:       "ROSA",
						Identifier: "rosa",
						Teams: []config.Team{
							{
								Name:       "Aurora",
								Identifier: "rosa-aurora",
								Ownership: config.Ownership{
									Method:         "team_field",
									Project:        "SREP",
									TeamFieldValue: "Aurora",
								},
							},
							{
								Name:       "Coffee",
								Identifier: "rosa-coffee",
								Ownership: config.Ownership{
									Method:         "team_field",
									Project:        "OCM",
									TeamFieldValue: "Coffee",
								},
							},
						},
					},
					{
						Name:       "Fleet",
						Identifier: "fleet",
						Teams: []config.Team{
							{
								Name:       "Fleet Console",
								Identifier: "fleet-console",
								Ownership: config.Ownership{
									Method:     "component",
									Project:    "OCMUI",
									Components: []string{"ocm-console"},
								},
							},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Resolver", func() {

	Describe("bare name resolution", func() {
		It("resolves a unique organization identifier", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("hcm")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Level).To(Equal(config.LevelOrganization))
			Expect(entity.Organization.Identifier).To(Equal("hcm"))
			Expect(entity.Path).To(Equal("hcm"))
		})

		It("resolves a unique pillar identifier", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("rosa")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Level).To(Equal(config.LevelPillar))
			Expect(entity.Pillar.Identifier).To(Equal("rosa"))
			Expect(entity.Organization.Identifier).To(Equal("hcm"))
			Expect(entity.Path).To(Equal("hcm/rosa"))
		})

		It("resolves a unique team identifier", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("rosa-aurora")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Level).To(Equal(config.LevelTeam))
			Expect(entity.Team.Identifier).To(Equal("rosa-aurora"))
			Expect(entity.Pillar.Identifier).To(Equal("rosa"))
			Expect(entity.Organization.Identifier).To(Equal("hcm"))
			Expect(entity.Path).To(Equal("hcm/rosa/rosa-aurora"))
		})

		It("returns not-found error for unknown identifier", func() {
			rm := makeResourceMapForResolver()
			_, err := rm.Resolve("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("slash-delimited path resolution", func() {
		It("resolves pillar/team path", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("rosa/rosa-aurora")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Level).To(Equal(config.LevelTeam))
			Expect(entity.Team.Identifier).To(Equal("rosa-aurora"))
			Expect(entity.Pillar.Identifier).To(Equal("rosa"))
		})

		It("resolves org/pillar path", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("hcm/rosa")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Level).To(Equal(config.LevelPillar))
			Expect(entity.Pillar.Identifier).To(Equal("rosa"))
			Expect(entity.Organization.Identifier).To(Equal("hcm"))
		})

		It("resolves org/pillar/team path", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("hcm/rosa/rosa-aurora")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Level).To(Equal(config.LevelTeam))
			Expect(entity.Team.Identifier).To(Equal("rosa-aurora"))
			Expect(entity.Path).To(Equal("hcm/rosa/rosa-aurora"))
		})

		It("returns not-found for invalid three-part path", func() {
			rm := makeResourceMapForResolver()
			_, err := rm.Resolve("hcm/rosa/nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("returns not-found for invalid two-part path", func() {
			rm := makeResourceMapForResolver()
			_, err := rm.Resolve("rosa/nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("returns error for too many segments", func() {
			rm := makeResourceMapForResolver()
			_, err := rm.Resolve("a/b/c/d")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at most 3 segments"))
		})
	})

	Describe("Teams()", func() {
		It("returns all teams across all pillars for an organization", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("hcm")
			Expect(err).NotTo(HaveOccurred())
			teams := entity.Teams()
			Expect(teams).To(HaveLen(3))
			identifiers := []string{teams[0].Identifier, teams[1].Identifier, teams[2].Identifier}
			Expect(identifiers).To(ConsistOf("rosa-aurora", "rosa-coffee", "fleet-console"))
		})

		It("returns all teams in the pillar for a pillar entity", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("rosa")
			Expect(err).NotTo(HaveOccurred())
			teams := entity.Teams()
			Expect(teams).To(HaveLen(2))
			identifiers := []string{teams[0].Identifier, teams[1].Identifier}
			Expect(identifiers).To(ConsistOf("rosa-aurora", "rosa-coffee"))
		})

		It("returns a single-element slice for a team entity", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("rosa-aurora")
			Expect(err).NotTo(HaveOccurred())
			teams := entity.Teams()
			Expect(teams).To(HaveLen(1))
			Expect(teams[0].Identifier).To(Equal("rosa-aurora"))
		})
	})

	Describe("hierarchy context", func() {
		It("includes organization info when resolving a team", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("rosa-coffee")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Organization.Name).To(Equal("Hybrid Cloud Management"))
			Expect(entity.Pillar.Name).To(Equal("ROSA"))
			Expect(entity.Team.Name).To(Equal("Coffee"))
		})

		It("includes organization info when resolving a pillar", func() {
			rm := makeResourceMapForResolver()
			entity, err := rm.Resolve("fleet")
			Expect(err).NotTo(HaveOccurred())
			Expect(entity.Organization.Name).To(Equal("Hybrid Cloud Management"))
			Expect(entity.Pillar.Name).To(Equal("Fleet"))
		})
	})
})
