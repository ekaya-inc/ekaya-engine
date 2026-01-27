# Issue: Question Status Transitions Not Protected

## Observed Behavior

The `skip_ontology_question` tool successfully changes a question's status from "answered" to "skipped". The original answer is retained but the status is overwritten.

## Expected Behavior

Once a question is resolved (status="answered"), subsequent status change operations should either:
1. Return an error indicating the question is already resolved
2. Reject the operation without changing status

## Steps to Reproduce

1. Call `mcp__mcp_test_suite__resolve_ontology_question` with:
   ```json
   {
     "question_id": "350bda6a-a906-46fb-a9e4-de421cf3d1d7",
     "resolution_notes": "MCP test suite - resolved for testing purposes"
   }
   ```

2. Response returns:
   ```json
   {"question_id":"350bda6a-a906-46fb-a9e4-de421cf3d1d7","resolution_notes":"MCP test suite - resolved for testing purposes","resolved_at":"2026-01-27T18:17:38+02:00","status":"answered"}
   ```

3. Call `mcp__mcp_test_suite__skip_ontology_question` with:
   ```json
   {
     "question_id": "350bda6a-a906-46fb-a9e4-de421cf3d1d7",
     "reason": "Attempting to skip already-resolved question"
   }
   ```

4. Response returns success:
   ```json
   {"question_id":"350bda6a-a906-46fb-a9e4-de421cf3d1d7","reason":"Attempting to skip already-resolved question","skipped_at":"2026-01-27T18:17:43+02:00","status":"skipped"}
   ```

5. Question is now "skipped" instead of "answered"

## Context

- Project ID: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- MCP Server: `mcp_test_suite`
- Test: `260-question-management.md`, Test Case 7

## Impact

- Resolved questions can be inadvertently changed to other statuses
- Answer history may be lost or confused
- Workflow integrity not maintained

## Possibly Related

- Similar issues may exist for other status transitions (dismiss→skip, escalate→resolve, etc.)
- May need a state machine for valid status transitions
