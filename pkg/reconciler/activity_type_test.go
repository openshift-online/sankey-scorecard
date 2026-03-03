package reconciler_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var _ = Describe("ExtractActivityType", func() {
	fieldID := "customfield_12320040"

	It("extracts value from map with 'value' key", func() {
		unknowns := map[string]interface{}{
			fieldID: map[string]interface{}{
				"value": "Tech Debt",
				"self":  "https://jira.example.com/...",
				"id":    "12345",
			},
		}
		result := reconciler.ExtractActivityType(unknowns, fieldID)
		Expect(result).To(Equal("Tech Debt"))
	})

	It("extracts value when stored directly as string", func() {
		unknowns := map[string]interface{}{
			fieldID: "Security & Compliance",
		}
		result := reconciler.ExtractActivityType(unknowns, fieldID)
		Expect(result).To(Equal("Security & Compliance"))
	})

	It("returns empty string when field is missing", func() {
		unknowns := map[string]interface{}{
			"other_field": "value",
		}
		result := reconciler.ExtractActivityType(unknowns, fieldID)
		Expect(result).To(BeEmpty())
	})

	It("returns empty string when field is nil", func() {
		unknowns := map[string]interface{}{
			fieldID: nil,
		}
		result := reconciler.ExtractActivityType(unknowns, fieldID)
		Expect(result).To(BeEmpty())
	})

	It("returns empty string when unknowns map is nil", func() {
		result := reconciler.ExtractActivityType(nil, fieldID)
		Expect(result).To(BeEmpty())
	})

	It("returns empty string when fieldID is empty", func() {
		unknowns := map[string]interface{}{
			fieldID: map[string]interface{}{"value": "Test"},
		}
		result := reconciler.ExtractActivityType(unknowns, "")
		Expect(result).To(BeEmpty())
	})

	It("returns empty string for unexpected type", func() {
		unknowns := map[string]interface{}{
			fieldID: 42,
		}
		result := reconciler.ExtractActivityType(unknowns, fieldID)
		Expect(result).To(BeEmpty())
	})

	It("returns empty string when map has no 'value' key", func() {
		unknowns := map[string]interface{}{
			fieldID: map[string]interface{}{
				"id":   "123",
				"self": "https://example.com",
			},
		}
		result := reconciler.ExtractActivityType(unknowns, fieldID)
		Expect(result).To(BeEmpty())
	})
})
