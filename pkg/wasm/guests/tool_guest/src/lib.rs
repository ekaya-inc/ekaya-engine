use extism_pdk::*;
use serde::Deserialize;
use serde_json::Value;

#[host_fn]
extern "ExtismHost" {
    fn tool_invoke(input: String) -> String;
}

#[derive(Deserialize)]
struct ToolResponse {
    result: Option<Value>,
    is_error: Option<bool>,
    error: Option<String>,
}

/// Dispatches test scenarios based on input string.
/// Returns "PASS" on success or "FAIL: <reason>" on failure.
#[plugin_fn]
pub fn run(input: String) -> FnResult<String> {
    match input.as_str() {
        "test_echo" => test_echo(),
        "test_error_tool" => test_error_tool(),
        "test_unknown_tool" => test_unknown_tool(),
        _ => Ok(format!("FAIL: unknown test case: {}", input)),
    }
}

fn test_echo() -> FnResult<String> {
    let req = serde_json::json!({
        "tool": "echo_tool",
        "arguments": {"msg": "hello"}
    })
    .to_string();

    let resp_str = unsafe { tool_invoke(req)? };
    let resp: ToolResponse = serde_json::from_str(&resp_str)?;

    if let Some(err) = &resp.error {
        return Ok(format!("FAIL: got error: {}", err));
    }

    let result = resp.result.ok_or_else(|| Error::msg("missing result"))?;
    let echo = result
        .get("echo")
        .and_then(|v| v.as_str())
        .unwrap_or("");

    if echo != "hello" {
        return Ok(format!("FAIL: expected echo=hello, got echo={}", echo));
    }

    if resp.is_error == Some(true) {
        return Ok("FAIL: expected is_error=false".to_string());
    }

    Ok("PASS".to_string())
}

fn test_error_tool() -> FnResult<String> {
    let req = serde_json::json!({
        "tool": "error_tool",
        "arguments": {}
    })
    .to_string();

    let resp_str = unsafe { tool_invoke(req)? };
    let resp: ToolResponse = serde_json::from_str(&resp_str)?;

    if resp.is_error != Some(true) {
        return Ok("FAIL: expected is_error=true".to_string());
    }

    Ok("PASS".to_string())
}

fn test_unknown_tool() -> FnResult<String> {
    let req = serde_json::json!({
        "tool": "nonexistent",
        "arguments": {}
    })
    .to_string();

    let resp_str = unsafe { tool_invoke(req)? };
    let resp: ToolResponse = serde_json::from_str(&resp_str)?;

    if resp.error.is_none() {
        return Ok("FAIL: expected error for unknown tool".to_string());
    }

    Ok("PASS".to_string())
}
