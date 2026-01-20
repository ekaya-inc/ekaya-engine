# PLAN: Show Error When Project Credentials Key Doesn't Match

## Problem

When a datasource exists but was encrypted with a different `PROJECT_CREDENTIALS_KEY` than the one currently configured:

1. The datasource List API silently fails to decrypt the config
2. The UI shows no datasources (appears empty)
3. When the user tries to create a new datasource, they get "datasource already exists" error
4. There's no indication of what's wrong or how to fix it

This happens in multi-environment setups where different `.env` files or configs are used (e.g., different hostname:port combinations requiring separate config files).

## Background

- `PROJECT_CREDENTIALS_KEY` is used to encrypt/decrypt `datasource_config` in `engine_datasources`
- The key is SHA-256 hashed to derive an AES key (see `pkg/crypto/credentials.go`)
- Once credentials are stored with a key, that key cannot be changed without re-encrypting
- This is an admin-facing configuration, not end-user

## Current Behavior

In `pkg/services/datasource.go:List()`:
```go
for i, ds := range datasources {
    config, err := s.decryptConfig(encryptedConfigs[i])
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt config for datasource %s: %w", ds.ID, err)
    }
    ds.Config = config
}
```

This returns an error that propagates to a 500 Internal Server Error, but the error message isn't helpful to the admin.

## Proposed Solution

### Backend Changes

1. **Create a new error type** in `pkg/apperrors/errors.go`:
   ```go
   var ErrCredentialsKeyMismatch = errors.New("datasource credentials were encrypted with a different key")
   ```

2. **Modify `pkg/services/datasource.go:List()`** to detect decryption failures and return a structured response:
   - Instead of failing completely, return datasources with a flag indicating decryption failed
   - Include metadata about which datasources have key mismatches

3. **Add a new response structure** for datasources with decryption status:
   ```go
   type DatasourceWithStatus struct {
       *models.Datasource
       DecryptionFailed bool   `json:"decryption_failed,omitempty"`
       ErrorMessage     string `json:"error_message,omitempty"`
   }
   ```

4. **Update handler response** to include the error state so UI can display it

### Frontend Changes

1. **Update datasource list page** to detect `decryption_failed` flag

2. **Show admin-facing error banner** when decryption fails:
   - Clear explanation: "Datasource credentials were encrypted with a different PROJECT_CREDENTIALS_KEY"
   - Two action options:
     - **"Disconnect Datasource"** - removes the datasource so a new one can be configured
     - **"Use Different Key"** - explains they need to restart the server with the correct key

3. **Terminology**: Use "Disconnect" not "Delete" for the action button

### Error Detection

In `pkg/crypto/credentials.go`, the `Decrypt()` function returns errors when:
- Base64 decoding fails
- Ciphertext is too short
- GCM decryption fails (authentication tag mismatch)

The GCM auth failure is the key indicator of a wrong encryption key.

## Implementation Steps

1. [x] Add `ErrCredentialsKeyMismatch` to apperrors
2. [ ] Modify `crypto.CredentialEncryptor.Decrypt()` to wrap GCM errors with identifiable type
3. [ ] Update `DatasourceService.List()` to catch key mismatch and return partial results with status
4. [ ] Update `DatasourcesHandler.List()` response to include decryption status
5. [ ] Update frontend `DatasourcePage` to detect and display the error
6. [ ] Add "Disconnect Datasource" action that calls DELETE endpoint
7. [ ] Add informational text about restarting with correct key
8. [ ] Write tests for the error detection and handling

## Files to Modify

### Backend
- `pkg/apperrors/errors.go` - add new error type
- `pkg/crypto/credentials.go` - wrap decryption errors with identifiable type
- `pkg/services/datasource.go` - handle partial decryption failures
- `pkg/handlers/datasources.go` - update response structure
- `pkg/handlers/datasources_test.go` - add tests

### Frontend
- `ui/src/pages/DatasourcePage.tsx` - display error and actions
- `ui/src/types/datasource.ts` - add status fields to type (if exists)

## Testing

1. Create a datasource with key A
2. Restart server with key B
3. Verify:
   - List endpoint returns datasource with `decryption_failed: true`
   - UI shows error banner with explanation
   - "Disconnect" button calls DELETE and refreshes
   - After disconnect, can create new datasource

## Notes

- This is an admin/developer-facing feature, not end-user
- The error should be clear but not expose sensitive details
- Consider logging the mismatch at WARN level for debugging
