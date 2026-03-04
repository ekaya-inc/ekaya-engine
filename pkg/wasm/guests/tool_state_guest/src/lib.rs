use extism_pdk::*;
use serde::Deserialize;
use serde_json::Value;

#[host_fn]
extern "ExtismHost" {
    fn tool_invoke(input: String) -> String;
    fn state_get(input: String) -> String;
    fn state_set(input: String) -> String;
}

#[derive(Deserialize)]
struct ToolResponse {
    result: Option<Value>,
    is_error: Option<bool>,
    error: Option<String>,
}

#[derive(Deserialize)]
struct StateGetResponse {
    data: Option<Value>,
    version: i64,
}

#[derive(Deserialize)]
struct StateSetResponse {
    version: Option<i64>,
    error: Option<String>,
}

/// Proves that tool_invoke, state_get, and state_set all work together
/// in a single WASM invocation.
///
/// Flow:
/// 1. Call echo_tool via tool_invoke → get result
/// 2. Store the tool result in state via state_set
/// 3. Read state back via state_get → verify it matches
#[plugin_fn]
pub fn run(_input: String) -> FnResult<String> {
    // 1. Call echo_tool via tool_invoke.
    let req = serde_json::json!({
        "tool": "echo_tool",
        "arguments": {"msg": "composed"}
    })
    .to_string();

    let resp_str = unsafe { tool_invoke(req)? };
    let resp: ToolResponse = serde_json::from_str(&resp_str)?;

    if let Some(err) = &resp.error {
        return Ok(format!("FAIL: tool_invoke error: {}", err));
    }

    let tool_result = resp.result.ok_or_else(|| Error::msg("missing result"))?;

    // 2. Store tool result in app state.
    let set_input = serde_json::json!({
        "data": tool_result,
        "version": 0
    })
    .to_string();

    let resp_str = unsafe { state_set(set_input)? };
    let set_resp: StateSetResponse = serde_json::from_str(&resp_str)?;

    if let Some(e) = &set_resp.error {
        return Ok(format!("FAIL: state_set error: {}", e));
    }
    if set_resp.version != Some(1) {
        return Ok(format!(
            "FAIL: expected state version 1, got {:?}",
            set_resp.version
        ));
    }

    // 3. Read state back and verify.
    let resp_str = unsafe { state_get(String::new())? };
    let get_resp: StateGetResponse = serde_json::from_str(&resp_str)?;

    let expected = serde_json::json!({"echo": "composed"});
    if get_resp.data.as_ref() != Some(&expected) {
        return Ok(format!(
            "FAIL: state data mismatch: expected {:?}, got {:?}",
            expected, get_resp.data
        ));
    }

    Ok("PASS".to_string())
}
