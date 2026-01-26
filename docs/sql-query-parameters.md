# SQL Query Parameter Template Syntax

## Overview

SQL query templates in ekaya-engine use the `{{parameter_name}}` syntax to mark parameter placeholders. This syntax is distinct from PostgreSQL's positional parameters (`$1`, `$2`, etc.) and shell variable syntax (`${var}`). The template syntax provides a named parameter system that is:

- **Human-readable and self-documenting** - Parameter names describe their purpose
- **Secure by design** - Parameters are never interpolated directly into SQL
- **Compatible with existing SQL** - Works with standard SQL syntax
- **Easy to parse and validate** - Simple regex pattern for extraction

## Template Syntax

Parameters are denoted using double curly braces with the parameter name inside:

```
{{parameter_name}}
```

### Parameter Name Rules

Parameter names must:
- Start with a letter or underscore
- Contain only alphanumeric characters and underscores (`a-z`, `A-Z`, `0-9`, `_`)
- Match the regex pattern: `[a-zA-Z_]\w*` (letter or underscore, followed by zero or more word characters)

### Valid Examples

```
{{user_id}}
{{start_date}}
{{max_price}}
{{_private_param}}
{{productID}}
```

### Invalid Examples

```
{{user-id}}       // Hyphens not allowed
{{123_param}}     // Cannot start with number
{{user id}}       // Spaces not allowed
{{user.id}}       // Dots not allowed
```

## Why `{{parameter_name}}`?

This syntax was chosen over alternatives for the following reasons:

### 1. Distinct from PostgreSQL's Positional Parameters

PostgreSQL uses `$1`, `$2`, etc. for parameter binding. Using a different syntax:
- Avoids confusion with the database's native parameter binding
- Allows named parameters which are more maintainable than positional ones
- Enables template processing before database execution

### 2. Distinct from Shell Variable Syntax

Shell variables use `${var}` syntax. Using double braces:
- Prevents accidental shell variable expansion
- Reduces risk of environment variable leakage
- Makes it clear these are SQL template parameters, not shell variables

### 3. Familiar from Popular Templating Systems

The `{{variable}}` syntax is used by:
- Mustache templates
- Handlebars.js
- Go's `text/template` package
- Jinja2 (with different default delimiters)

This familiarity makes it easy for developers to recognize and understand.

### 4. Easy to Parse

- Simple regex pattern: `\{\{([a-zA-Z_]\w*)\}\}`
- No complex escaping rules needed
- Fast extraction and validation

### 5. Visually Distinct

Double braces stand out in SQL code, making parameters easy to:
- Spot during code review
- Identify in error messages
- Understand in documentation

## SQL Template Examples

### Basic Query with Single Parameter

```sql
SELECT customer_name, email, created_at
FROM customers
WHERE id = {{customer_id}}
```

**Parameter Definition:**
```json
{
  "name": "customer_id",
  "type": "uuid",
  "description": "The unique identifier of the customer",
  "required": true
}
```

### Query with Multiple Parameters

```sql
SELECT customer_name, order_total, order_date
FROM orders o
JOIN customers c ON o.customer_id = c.id
WHERE c.id = {{customer_id}}
  AND o.order_date >= {{start_date}}
  AND o.order_date < {{end_date}}
ORDER BY o.order_date DESC
LIMIT {{limit}}
```

**Parameter Definitions:**
```json
[
  {
    "name": "customer_id",
    "type": "uuid",
    "description": "The customer's unique identifier",
    "required": true
  },
  {
    "name": "start_date",
    "type": "date",
    "description": "Start of the date range (inclusive)",
    "required": true
  },
  {
    "name": "end_date",
    "type": "date",
    "description": "End of the date range (exclusive)",
    "required": true
  },
  {
    "name": "limit",
    "type": "integer",
    "description": "Maximum number of results to return",
    "required": false,
    "default": 100
  }
]
```

### Query with Array Parameter

```sql
SELECT product_name, category, price
FROM products
WHERE category IN ({{categories}})
  AND price BETWEEN {{min_price}} AND {{max_price}}
ORDER BY price ASC
```

**Parameter Definitions:**
```json
[
  {
    "name": "categories",
    "type": "string[]",
    "description": "List of product categories to include",
    "required": true
  },
  {
    "name": "min_price",
    "type": "decimal",
    "description": "Minimum price (inclusive)",
    "required": false,
    "default": 0.00
  },
  {
    "name": "max_price",
    "type": "decimal",
    "description": "Maximum price (inclusive)",
    "required": false,
    "default": 999999.99
  }
]
```

### Query with Same Parameter Used Multiple Times

```sql
SELECT *
FROM transactions
WHERE (sender_id = {{user_id}} OR receiver_id = {{user_id}})
  AND amount > {{min_amount}}
```

When the same parameter appears multiple times, it is bound to the same value across all occurrences. The parameter will be assigned a single positional parameter (`$1`) that is reused in the prepared statement.

**After Substitution:**
```sql
SELECT *
FROM transactions
WHERE (sender_id = $1 OR receiver_id = $1)  -- Same $1 for both occurrences
  AND amount > $2
```

### Complex Query with Optional Filters

```sql
SELECT
  u.username,
  u.email,
  COUNT(o.id) AS order_count,
  SUM(o.total) AS total_spent
FROM users u
LEFT JOIN orders o ON u.id = o.user_id
WHERE u.status = {{status}}
  AND u.created_at >= {{start_date}}
  AND u.created_at < {{end_date}}
  AND ({{email_filter}} IS NULL OR u.email LIKE {{email_filter}})
GROUP BY u.id
HAVING COUNT(o.id) >= {{min_order_count}}
ORDER BY total_spent DESC
LIMIT {{limit}} OFFSET {{offset}}
```

## Execution Flow

When a parameterized query is executed, the following steps occur:

### 1. Template Parsing

Extract all `{{param}}` placeholders from the SQL template and build a list of unique parameter names in order of first appearance.

### 2. Parameter Validation

- Check that all parameters used in the template are defined
- Verify required parameters have values supplied
- Apply default values for optional parameters if no value supplied

### 3. Type Coercion

- Convert supplied values to their declared types
- Validate type compatibility (e.g., `"123"` → `int64`, `"2024-01-15"` → `date`)

### 4. Injection Detection

- Scan string parameter values using libinjection
- Reject queries if SQL injection patterns detected
- Log injection attempts to security audit log

### 5. Parameter Substitution

- Replace `{{param}}` with PostgreSQL positional parameters (`$1`, `$2`, etc.)
- Build ordered array of parameter values matching positional indices
- Handle reused parameters efficiently (same `$N` for same name)

### 6. Query Execution

- Execute prepared statement with positional parameters
- Return results to caller

## Example: Substitution Process

**Template SQL:**
```sql
SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}
```

**Parameter Definitions:**
```json
[
  {
    "name": "customer_id",
    "type": "uuid",
    "required": true
  },
  {
    "name": "min_total",
    "type": "decimal",
    "required": false,
    "default": 0.00
  }
]
```

**Supplied Values:**
```json
{
  "customer_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**After Substitution:**
```sql
SELECT * FROM orders WHERE customer_id = $1 AND total > $2
```

**Ordered Parameter Array:**
```go
[]any{
    "550e8400-e29b-41d4-a716-446655440000",  // $1
    0.00,                                     // $2 (default applied)
}
```

## Supported Parameter Types

| Type        | Go Type      | PostgreSQL Type | Example Value                          |
|-------------|--------------|-----------------|----------------------------------------|
| `string`    | `string`     | `text`          | `"hello world"`                        |
| `integer`   | `int64`      | `bigint`        | `123`                                  |
| `decimal`   | `float64`    | `numeric`       | `99.95`                                |
| `boolean`   | `bool`       | `boolean`       | `true`                                 |
| `date`      | `string` (ISO) | `date`        | `"2024-01-15"`                         |
| `timestamp` | `string` (ISO) | `timestamptz` | `"2024-01-15T10:30:00Z"`               |
| `uuid`      | `string`     | `uuid`          | `"550e8400-e29b-41d4-a716-446655440000"` |
| `string[]`  | `[]string`   | `text[]`        | `["apple", "banana", "cherry"]`        |
| `integer[]` | `[]int64`    | `bigint[]`      | `[1, 2, 3, 4, 5]`                      |

## Best Practices

### 1. Use Descriptive Parameter Names

**Good:**
```sql
WHERE created_at >= {{start_date}} AND created_at < {{end_date}}
```

**Bad:**
```sql
WHERE created_at >= {{d1}} AND created_at < {{d2}}
```

### 2. Document Parameters Clearly

Always provide clear descriptions for parameters:

```json
{
  "name": "customer_id",
  "type": "uuid",
  "description": "The unique identifier of the customer",
  "required": true
}
```

### 3. Use Array Parameters for IN Clauses

Instead of building dynamic SQL:

```sql
WHERE status IN ({{statuses}})
```

With parameter definition:
```json
{
  "name": "statuses",
  "type": "string[]",
  "description": "List of order statuses to include",
  "required": true
}
```

### 4. Provide Sensible Defaults for Optional Parameters

```json
{
  "name": "limit",
  "type": "integer",
  "description": "Maximum number of results to return",
  "required": false,
  "default": 100
}
```

### 5. Use Type-Specific Parameters

Instead of generic string parameters, use specific types for better validation:

**Good:**
```sql
WHERE order_date = {{order_date}}  -- type: "date"
```

**Bad:**
```sql
WHERE order_date = {{order_date}}  -- type: "string"
```

The `date` type provides validation that the string is a valid ISO date format.

## Limitations and Constraints

### 1. No Dynamic Table or Column Names

Parameter substitution only works for **VALUES** in WHERE clauses and similar contexts.

**NOT SUPPORTED:**

```sql
SELECT * FROM {{table_name}}           -- Table names
SELECT {{column_name}} FROM orders     -- Column names
SELECT * FROM orders {{order_clause}}  -- SQL keywords
```

This is a **security feature** - dynamic table/column names require query rewriting and cannot be safely parameterized.

### 2. No Complex Expressions

Parameters must be complete values:

**Good:**
```sql
WHERE price > {{min_price}}
```

**Bad:**
```sql
WHERE price > {{min_price}} * 1.1
```

Embed the calculation in the parameter value instead.

### 3. Array Parameters Require Array Types

Array parameters must be declared with array types and used in array contexts:

```json
{
  "name": "product_ids",
  "type": "integer[]",
  "required": true
}
```

```sql
WHERE product_id = ANY({{product_ids}})
-- OR
WHERE product_id IN ({{product_ids}})
```

### 4. Parameter Names Must Be Valid Identifiers

- Must start with letter or underscore: `{{_private}}` ✓, `{{123}}` ✗
- Can contain alphanumeric and underscore: `{{user_id_2}}` ✓, `{{user-id}}` ✗
- Case-sensitive: `{{userId}}` and `{{userid}}` are different parameters

## Security Model

The template syntax is part of a defense-in-depth security model:

### 1. Named Parameters
Parameters are never interpolated as raw SQL strings. All values are bound using PostgreSQL's parameterized query mechanism.

### 2. Type Enforcement
Parameter types are declared and validated before execution. Type coercion happens in application code, not in SQL.

### 3. Injection Detection
All string parameters are scanned with libinjection. Detection happens before query execution. Suspicious values are rejected and logged.

### 4. Query Approval
MCP clients can only execute pre-approved query templates. Users cannot execute arbitrary SQL. Templates are reviewed and validated before enabling.

### 5. Audit Logging
- All query executions logged with parameter values
- Injection attempts logged for SIEM integration
- Security events include client IP and fingerprints

## Error Messages

Common validation errors and their meanings:

| Error | Meaning |
|-------|---------|
| `parameter {{user_id}} used in SQL but not defined` | The SQL template contains `{{user_id}}` but no parameter definition exists |
| `parameter 'email' is required but no value was supplied` | The parameter definition has `required=true` but no value was provided |
| `potential SQL injection detected in parameter 'search_query'` | The supplied value matches SQL injection patterns |
| `invalid type for parameter 'count': expected integer, got string` | Type coercion failed because the value can't be converted to the declared type |

## Related Documentation

- [PLAN-parameterized-queries.md](../PLAN-parameterized-queries.md) - Full implementation plan
- [pkg/sql/parameter_syntax.go](../pkg/sql/parameter_syntax.go) - Detailed syntax documentation
- [pkg/sql/parameters.go](../pkg/sql/parameters.go) - Implementation (Phase 4)
- [pkg/sql/injection.go](../pkg/sql/injection.go) - Injection detection (Phase 5)

## Implementation Reference

The parameter template syntax is implemented in the following components:

- `ExtractParameters(sql)` - Finds all `{{param}}` placeholders
- `ValidateParameterDefinitions(sql, params)` - Validates definitions match usage
- `SubstituteParameters(sql, defs, values)` - Replaces `{{param}}` with `$N`
- `CheckParameterForInjection(name, value)` - Detects SQL injection attempts

See [pkg/sql/parameters.go](../pkg/sql/parameters.go) for implementation details.
