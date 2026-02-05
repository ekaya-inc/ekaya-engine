# Issue: Cancelling Ontology Extraction Causes Server-Side Errors

**Status:** OPEN (Low Priority) - Cosmetic issue, extraction cancellation works correctly.

## Observed Behavior

When a user cancels an in-progress ontology extraction via the UI "Cancel" button, multiple errors with stack traces appear in `output.log`.

This is normal, expected user behavior - users should be able to cancel long-running extractions without causing server-side errors.

## Steps to Reproduce

1. Start an ontology extraction on a project with meaningful data
2. While extraction is running (e.g., during "Discovering Primary Key Matches" step)
3. Click the "Cancel" button in the UI
4. Observe `output.log` - multiple errors with stack traces appear

## Expected Behavior

Cancellation should be handled gracefully:
- No errors or stack traces in server logs
- Clean shutdown of in-progress operations
- Appropriate log level (INFO or DEBUG) for cancellation notices

## Notes

- The extraction does stop successfully from the user's perspective
- The DAG status correctly shows "cancelled" in the UI
- The issue is purely about noisy/alarming server logs for a normal operation
