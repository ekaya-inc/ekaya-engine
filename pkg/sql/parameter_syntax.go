// Package sql provides SQL validation and parameter templating utilities.
package sql

/*
Parameter Template Syntax Documentation

# Overview

SQL query templates in ekaya-engine use the {{parameter_name}} syntax to mark parameter
placeholders. This syntax is distinct from PostgreSQL's positional parameters ($1, $2, etc.)
and shell variable syntax (${var}). The template syntax provides a named parameter system
that is:

- Human-readable and self-documenting
- Secure by design (parameters are never interpolated directly)
- Compatible with existing SQL syntax
- Easy to parse and validate

# Template Syntax

Parameters are denoted using double curly braces with the parameter name inside:

	{{parameter_name}}

Parameter names must:
- Start with a letter or underscore
- Contain only alphanumeric characters and underscores (a-z, A-Z, 0-9, _)
- Match the regex pattern: [a-zA-Z_]\w* (letter or underscore, followed by zero or more word characters)

# Why {{parameter_name}}?

This syntax was chosen over alternatives for the following reasons:

1. Distinct from PostgreSQL's $1, $2 positional parameters
   - Avoids confusion with the database's native parameter binding syntax
   - Named parameters are more maintainable than positional ones

2. Distinct from shell variable syntax ${var}
   - Prevents accidental shell variable expansion
   - Reduces risk of environment variable leakage

3. Familiar from popular templating systems
   - Similar to Mustache, Handlebars, and Go's text/template
   - Easy for developers to recognize and understand

4. Easy to parse with regex
   - Simple pattern: \{\{([a-zA-Z_]\w*)\}\}
   - No complex escaping rules needed

5. Visually distinct
   - Double braces stand out in SQL code
   - Makes parameters easy to spot during code review

# SQL Template Examples

## Basic Query with Single Parameter

	SELECT customer_name, email, created_at
	FROM customers
	WHERE id = {{customer_id}}

## Query with Multiple Parameters

	SELECT customer_name, order_total, order_date
	FROM orders o
	JOIN customers c ON o.customer_id = c.id
	WHERE c.id = {{customer_id}}
	  AND o.order_date >= {{start_date}}
	  AND o.order_date < {{end_date}}
	ORDER BY o.order_date DESC
	LIMIT {{limit}}

## Query with Array Parameter

	SELECT product_name, category, price
	FROM products
	WHERE category IN ({{categories}})
	  AND price BETWEEN {{min_price}} AND {{max_price}}
	ORDER BY price ASC

## Query with Same Parameter Used Multiple Times

	SELECT *
	FROM transactions
	WHERE (sender_id = {{user_id}} OR receiver_id = {{user_id}})
	  AND amount > {{min_amount}}

When the same parameter appears multiple times, it is bound to the same value across
all occurrences. The parameter will be assigned a single positional parameter ($1)
that is reused in the prepared statement.

## Complex Query with Optional Parameters

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

# Execution Flow

When a parameterized query is executed:

1. Template Parsing
   - Extract all {{param}} placeholders from the SQL template
   - Build a list of unique parameter names in order of first appearance

2. Parameter Validation
   - Check that all parameters used in the template are defined
   - Verify required parameters have values supplied
   - Apply default values for optional parameters if no value supplied

3. Type Coercion
   - Convert supplied values to their declared types
   - Validate type compatibility (e.g., "123" → int64, "2024-01-15" → date)

4. Injection Detection
   - Scan string parameter values using libinjection
   - Reject queries if SQL injection patterns detected
   - Log injection attempts to security audit log

5. Parameter Substitution
   - Replace {{param}} with PostgreSQL positional parameters ($1, $2, etc.)
   - Build ordered array of parameter values matching positional indices
   - Handle reused parameters efficiently (same $N for same name)

6. Query Execution
   - Execute prepared statement with positional parameters
   - Return results to caller

# Example: Substitution Process

Template SQL:
	SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}

With parameter definitions:
	[
	  {name: "customer_id", type: "uuid", required: true},
	  {name: "min_total", type: "decimal", required: false, default: 0.00}
	]

And supplied values:
	{
	  "customer_id": "550e8400-e29b-41d4-a716-446655440000"
	}

After substitution:
	SELECT * FROM orders WHERE customer_id = $1 AND total > $2

With ordered parameter array:
	[
	  "550e8400-e29b-41d4-a716-446655440000",  // $1
	  0.00                                      // $2 (default applied)
	]

# Security Model

The template syntax is part of a defense-in-depth security model:

1. Named Parameters
   - Parameters are never interpolated as raw SQL strings
   - All values are bound using PostgreSQL's parameterized query mechanism

2. Type Enforcement
   - Parameter types are declared and validated before execution
   - Type coercion happens in application code, not in SQL

3. Injection Detection
   - All string parameters scanned with libinjection
   - Detection happens before query execution
   - Suspicious values rejected and logged

4. Query Approval
   - MCP clients can only execute pre-approved query templates
   - Users cannot execute arbitrary SQL
   - Templates are reviewed and validated before enabling

5. Audit Logging
   - All query executions logged with parameter values
   - Injection attempts logged for SIEM integration
   - Security events include client IP and fingerprints

# Supported Parameter Types

The following parameter types are supported:

| Type        | Go Type      | PostgreSQL Type | Example Value                        |
|-------------|--------------|----------------|--------------------------------------|
| string      | string       | text           | "hello world"                        |
| integer     | int64        | bigint         | 123                                  |
| decimal     | float64      | numeric        | 99.95                                |
| boolean     | bool         | boolean        | true                                 |
| date        | string (ISO) | date           | "2024-01-15"                         |
| timestamp   | string (ISO) | timestamptz    | "2024-01-15T10:30:00Z"               |
| uuid        | string       | uuid           | "550e8400-e29b-41d4-a716-446655440000" |
| string[]    | []string     | text[]         | ["apple", "banana", "cherry"]        |
| integer[]   | []int64      | bigint[]       | [1, 2, 3, 4, 5]                      |

# Common Patterns and Best Practices

## 1. Use Descriptive Parameter Names

Good:
	WHERE created_at >= {{start_date}} AND created_at < {{end_date}}

Bad:
	WHERE created_at >= {{d1}} AND created_at < {{d2}}

## 2. Document Parameters in Query Metadata

Always provide clear descriptions for parameters:

	{
	  "name": "customer_id",
	  "type": "uuid",
	  "description": "The unique identifier of the customer",
	  "required": true
	}

## 3. Use Array Parameters for IN Clauses

Instead of dynamic SQL generation:
	WHERE status IN ({{statuses}})

Parameter definition:
	{
	  "name": "statuses",
	  "type": "string[]",
	  "description": "List of order statuses to include",
	  "required": true
	}

## 4. Provide Sensible Defaults for Optional Parameters

	{
	  "name": "limit",
	  "type": "integer",
	  "description": "Maximum number of results to return",
	  "required": false,
	  "default": 100
	}

## 5. Use Type-Specific Parameters for Better Validation

Instead of generic string parameters:

	WHERE order_date = {{order_date}}  // type: "date"

Not:

	WHERE order_date = {{order_date}}  // type: "string"

The date type provides validation that the string is a valid ISO date format.

# Limitations and Constraints

## 1. No Dynamic Table or Column Names

Parameter substitution only works for VALUES in WHERE clauses and similar contexts.
You CANNOT use parameters for:

- Table names: SELECT * FROM {{table_name}}  // NOT SUPPORTED
- Column names: SELECT {{column_name}} FROM orders  // NOT SUPPORTED
- SQL keywords: SELECT * FROM orders {{order_clause}}  // NOT SUPPORTED

This is a security feature - dynamic table/column names require query rewriting
and cannot be safely parameterized.

## 2. No Complex Expressions

Parameters must be complete values:

Good:
	WHERE price > {{min_price}}

Bad:
	WHERE price > {{min_price}} * 1.1  // Embed the calculation in the parameter value

## 3. Array Parameters Require Array Types

Array parameters must be declared with array types:

	{
	  "name": "product_ids",
	  "type": "integer[]",  // Must be array type
	  "required": true
	}

And used in array contexts:

	WHERE product_id = ANY({{product_ids}})

Or:

	WHERE product_id IN ({{product_ids}})

## 4. Parameter Names Must Be Valid Identifiers

Parameter names follow identifier rules:
- Must start with letter or underscore: {{_private}} ✓, {{123}} ✗
- Can contain alphanumeric and underscore: {{user_id_2}} ✓, {{user-id}} ✗
- Case-sensitive: {{userId}} and {{userid}} are different parameters

# Error Messages

Common validation errors and their meanings:

"parameter {{user_id}} used in SQL but not defined"
  → The SQL template contains {{user_id}} but no parameter definition exists

"parameter 'email' is required but no value was supplied"
  → The parameter definition has required=true but no value was provided

"potential SQL injection detected in parameter 'search_query'"
  → The supplied value for 'search_query' matches SQL injection patterns

"invalid type for parameter 'count': expected integer, got string"
  → Type coercion failed because the supplied value can't be converted to the declared type

"parameter 'table_name' cannot be used for dynamic identifiers"
  → Attempted to use parameter for table/column name, which is not supported

# Related Documentation

- pkg/sql/parameters.go - Implementation of parameter extraction and substitution
- pkg/sql/injection.go - SQL injection detection using libinjection
- pkg/services/query.go - QueryService.ExecuteWithParameters method
- PLAN-parameterized-queries.md - Full implementation plan and architecture

# Implementation Reference

The parameter template syntax is implemented in the following components:

- ExtractParameters(sql) - Finds all {{param}} placeholders
- ValidateParameterDefinitions(sql, params) - Validates definitions match usage
- SubstituteParameters(sql, defs, values) - Replaces {{param}} with $N
- CheckParameterForInjection(name, value) - Detects SQL injection attempts

See pkg/sql/parameters.go for implementation details.
*/
