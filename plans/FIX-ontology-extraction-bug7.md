# FIX: BUG-7 - Generic SaaS Glossary Terms

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-7
**Severity:** Medium
**Category:** Glossary
**Related:** BUG-5 (Missing Critical Domain Knowledge)

## Problem Summary

The glossary contains generic SaaS business metrics that don't reflect Tikr's business model:

| Generated Term | Issue |
|----------------|-------|
| Active Subscribers | Tikr uses pay-per-use engagements, not subscriptions |
| Churn Rate | Doesn't apply to pay-per-use model |
| Customer Lifetime Value | Calculation won't work with engagement-based billing |
| Average Order Value | Tikr has engagements, not orders |
| Inventory Turnover | Tikr has no inventory |

### Expected Terms for Tikr

- **Tik** - A 6-second unit of billed engagement time
- **Engagement** - A paid video session between Host and Visitor
- **Earned Amount** - Host's earnings after platform fees and Tikr share
- **Tikr Share** - Platform's revenue share (~30% after fees)
- **Platform Fees** - Payment processing fees (~4.5%)
- **Preauthorization** - Hold on Visitor's card before engagement starts

## Root Cause

**The glossary prompt explicitly suggests generic SaaS metrics.**

**File:** `pkg/services/glossary_service.go:445-462`

```go
func (s *glossaryService) suggestTermsSystemMessage() string {
    return `You are a business analyst expert...

Focus on discovering as many useful business terms as possible across these categories:
1. Key Performance Indicators (KPIs) - metrics that measure business success
2. Financial metrics - revenue, costs, margins, GMV, AOV, etc.  ← GENERIC!
3. User/customer metrics - active users, retention, churn, lifetime value, etc.  ← SUBSCRIPTION!
4. Transaction metrics - volume, value, conversion rates, etc.
5. Engagement metrics - sessions, page views, time on platform, etc.
6. Growth metrics - new users, growth rates, acquisition costs, etc.
...`
}
```

The prompt:
1. **Hardcodes generic metric categories** (GMV, AOV, churn, lifetime value)
2. **Doesn't consider project-specific domain knowledge**
3. **Doesn't instruct LLM to avoid inapplicable patterns**

## Fix Implementation

### 1. Make Metric Categories Domain-Aware

**File:** `pkg/services/glossary_service.go`

```go
func (s *glossaryService) suggestTermsSystemMessage() string {
    return `You are a business analyst expert. Your task is to analyze a database schema and identify business metrics and terms SPECIFIC TO THIS DOMAIN.

IMPORTANT: Only suggest metrics that apply to the specific business model shown in the schema.
- DO NOT suggest subscription metrics if the model is pay-per-use
- DO NOT suggest inventory metrics if there is no inventory
- DO NOT suggest e-commerce metrics (AOV, GMV) if there are no orders/products

Analyze the entity names and descriptions to understand the business model before suggesting terms.

For each term, provide:
- term: A clear business name specific to this domain
- definition: What it measures and how it's calculated
- aliases: Alternative names business users might use
`
}
```

### 2. Include Project Knowledge in Prompt

**File:** `pkg/services/glossary_service.go:465`

```go
func (s *glossaryService) buildSuggestTermsPrompt(ontology *models.TieredOntology, entities []*models.OntologyEntity, projectKnowledge []models.KnowledgeFact) string {
    var sb strings.Builder

    // Include project knowledge FIRST to set context
    if len(projectKnowledge) > 0 {
        sb.WriteString("# Domain-Specific Knowledge\n\n")
        sb.WriteString("The following facts describe this specific business:\n\n")
        for _, fact := range projectKnowledge {
            sb.WriteString(fmt.Sprintf("- **%s**: %s\n", fact.FactType, fact.Key))
        }
        sb.WriteString("\nUse this knowledge to suggest domain-appropriate metrics.\n\n")
    }

    // ... rest of prompt
}
```

### 3. Add Negative Examples for Generic Terms

```go
sb.WriteString(`
## What NOT to Suggest

Do NOT suggest these generic terms unless the schema supports them:
- "Active Subscribers" - only for subscription businesses
- "Churn Rate" - only for subscription businesses
- "Customer Lifetime Value" - requires purchase history
- "Average Order Value" - requires order/cart system
- "Inventory Turnover" - requires inventory management

Instead, look for domain-specific metrics based on:
- What entities exist (Engagement, Transaction, Session, etc.)
- What columns track value (amount, fee, revenue)
- What time-based columns exist (duration, start_time, end_time)
`)
```

### 4. Domain-Specific Term Templates

Create template suggestions based on detected domain patterns:

```go
func (s *glossaryService) getDomainHints(entities []*models.OntologyEntity) []string {
    var hints []string

    // Detect patterns from entity names
    hasEngagement := containsEntity(entities, "engagement", "session")
    hasBilling := containsEntity(entities, "billing", "transaction", "payment")
    hasUserRoles := hasColumnsLike(entities, "host_id", "visitor_id", "creator_id")

    if hasEngagement && !containsEntity(entities, "subscription", "plan") {
        hints = append(hints, "This appears to be an engagement/session-based business, not subscription")
    }
    if hasBilling {
        hints = append(hints, "Focus on transaction-based metrics (revenue per engagement, fees, payouts)")
    }
    if hasUserRoles {
        hints = append(hints, "There are distinct user roles - consider role-specific metrics")
    }

    return hints
}
```

### 5. Post-Generation Validation

Filter out inapplicable terms after LLM generation:

```go
func (s *glossaryService) filterInapplicableTerms(
    terms []GlossaryTerm,
    entities []*models.OntologyEntity,
) []GlossaryTerm {
    // Terms that require specific entity types
    subscriptionTerms := []string{"subscriber", "subscription", "churn", "mrr", "arr"}
    inventoryTerms := []string{"inventory", "stock", "warehouse"}
    ecommerceTerms := []string{"order value", "cart", "checkout"}

    hasSubscription := containsEntity(entities, "subscription", "plan", "membership")
    hasInventory := containsEntity(entities, "inventory", "product", "stock")
    hasEcommerce := containsEntity(entities, "order", "cart", "checkout")

    var filtered []GlossaryTerm
    for _, term := range terms {
        termLower := strings.ToLower(term.Term)

        // Skip subscription terms if no subscription entities
        if !hasSubscription && matchesAny(termLower, subscriptionTerms) {
            continue
        }
        // Skip inventory terms if no inventory entities
        if !hasInventory && matchesAny(termLower, inventoryTerms) {
            continue
        }
        // Skip e-commerce terms if no e-commerce entities
        if !hasEcommerce && matchesAny(termLower, ecommerceTerms) {
            continue
        }

        filtered = append(filtered, term)
    }
    return filtered
}
```

## Connection to BUG-5

This bug is closely related to BUG-5 (Missing Critical Domain Knowledge). The fixes complement each other:

- **BUG-5 Fix**: Adds knowledge seeding mechanism to capture domain facts
- **BUG-7 Fix**: Makes glossary discovery use that knowledge to generate relevant terms

Implementing BUG-5's knowledge seeding will automatically improve BUG-7 if the glossary prompt includes project knowledge.

## Testing

1. Seed project knowledge with Tikr-specific facts
2. Run glossary discovery
3. Verify:
   - ✓ "Tik", "Engagement", "Earned Amount" suggested
   - ✗ "Active Subscribers", "Churn Rate" NOT suggested
   - ✗ "Inventory Turnover" NOT suggested

## Acceptance Criteria

- [ ] Glossary prompt doesn't hardcode generic SaaS metrics
- [ ] Project knowledge included in glossary discovery context
- [ ] LLM instructed to avoid inapplicable patterns
- [ ] Post-generation filtering removes obvious mismatches
- [ ] Domain-specific terms generated based on actual entities
