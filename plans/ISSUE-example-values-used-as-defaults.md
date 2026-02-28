# ISSUE: Parameter "example" Values Are Silently Used as Defaults for Optional Parameters

Status: FIXED
Severity: Medium
Area: MCP tools — pre-approved query parameter handling

## Observed

When an MCP client calls `execute_approved_query` and omits an optional parameter, the system substitutes the parameter's `example` value instead of NULL. This causes unintended data writes — columns that should remain NULL get populated with example/placeholder values.

### Reproduction

1. Create a query with an optional parameter that has an example value:
   ```
   create_approved_query(
     name: "Set Content Post URLs",
     sql: "UPDATE content_posts SET linkedin_url = COALESCE({{linkedin_url}}, linkedin_url), blog_url = COALESCE({{blog_url}}, blog_url) ...",
     parameters: [
       {name: "post_id", type: "integer", required: true, example: 1},
       {name: "linkedin_url", type: "string", required: false, example: "https://linkedin.com/posts/..."},
       {name: "blog_url", type: "string", required: false, example: "https://ekaya.ai/blog/..."}
     ]
   )
   ```

2. Execute the query providing only `linkedin_url`, omitting `blog_url`:
   ```
   execute_approved_query(
     query_id: "...",
     parameters: {post_id: 9, linkedin_url: "https://linkedin.com/posts/real-post-123"}
   )
   ```

3. Observe: `blog_url` is set to `"https://ekaya.ai/blog/..."` (the example value) instead of remaining NULL. The COALESCE in the SQL sees a non-NULL value and writes it to the row.

## Root Cause

In `pkg/mcp/tools/dev_queries.go:858-860`, the `example` field from the parameter definition is assigned directly to the `Default` field of the QueryParameter model:

```go
if example, ok := paramMap["example"]; ok {
    param.Default = example
}
```

Then in `pkg/sql/parameters.go:189-236`, when a parameter is not supplied, the default (which is actually the example) is used:

```go
value, supplied := suppliedValues[name]
if !supplied {
    value = def.Default  // example value used here
}
```

The `example` field serves two purposes that conflict:
1. **Validation**: Used for dry-run validation when creating/updating the query (legitimate)
2. **Runtime default**: Used as the actual value when the parameter is omitted (unintended for most cases)

## Impact

- **Silent data corruption**: Columns get placeholder strings like `"https://ekaya.ai/blog/..."` written to production data
- **COALESCE pattern breaks**: Many write queries use `COALESCE({{param}}, existing_column)` to support partial updates. This pattern assumes omitted params are NULL, but they arrive as example values, overwriting existing data
- **Hard to debug**: The MCP client (LLM) has no indication that omitted params are being substituted — it believes it's doing a partial update
- **Workaround burden**: MCP clients must explicitly pass NULL for every unused optional parameter, which defeats the purpose of optional params

## Suggested Fix

Separate `example` from `default` in the parameter model:

- `example`: Used only for dry-run validation when creating/saving the query. Never used at execution time.
- `default`: A separate, explicit field that the query creator intentionally sets when they want a fallback value at runtime. Most optional params would have no default (= NULL when omitted).

In `parseDevQueryParameterDefinitions`:
```go
// example is for validation only, not runtime
if example, ok := paramMap["example"]; ok {
    param.Example = example
}
// default is an explicit runtime fallback (separate from example)
if def, ok := paramMap["default"]; ok {
    param.Default = def
}
```

In `SubstituteParameters`:
```go
if !supplied {
    value = def.Default  // only uses explicitly set defaults, not examples
}
```

During query validation (create/update), use `param.Example` for the dry-run instead of `param.Default`.

### Migration

Existing queries that rely on the current behavior (example-as-default) would need review. Any query where the example value is also the intended runtime default should add an explicit `default` field.
