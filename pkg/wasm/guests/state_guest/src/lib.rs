use extism_pdk::*;
use serde::Deserialize;
use serde_json::Value;

#[host_fn]
extern "ExtismHost" {
    fn state_get(input: String) -> String;
    fn state_set(input: String) -> String;
}

#[derive(Deserialize)]
struct GetResponse {
    data: Option<Value>,
    version: i64,
}

#[derive(Deserialize)]
struct SetResponse {
    version: Option<i64>,
    error: Option<String>,
}

#[plugin_fn]
pub fn run(_input: String) -> FnResult<String> {
    // 1. Get initial state — expect null data, version 0.
    let resp = unsafe { state_get(String::new())? };
    let get1: GetResponse = serde_json::from_str(&resp)?;

    if get1.data.is_some() && get1.data.as_ref().unwrap() != &Value::Null {
        return Ok(format!("FAIL: expected null initial data, got {:?}", get1.data));
    }
    if get1.version != 0 {
        return Ok(format!("FAIL: expected initial version 0, got {}", get1.version));
    }

    // 2. Set state with data.
    let set_input = serde_json::json!({"data": {"key": "value"}, "version": 0}).to_string();
    let resp = unsafe { state_set(set_input)? };
    let set1: SetResponse = serde_json::from_str(&resp)?;

    if let Some(e) = &set1.error {
        return Ok(format!("FAIL: state_set returned error: {}", e));
    }
    if set1.version != Some(1) {
        return Ok(format!("FAIL: expected version 1 after set, got {:?}", set1.version));
    }

    // 3. Get state — expect our data back at version 1.
    let resp = unsafe { state_get(String::new())? };
    let get2: GetResponse = serde_json::from_str(&resp)?;

    let expected = serde_json::json!({"key": "value"});
    if get2.data.as_ref() != Some(&expected) {
        return Ok(format!("FAIL: expected data {:?}, got {:?}", expected, get2.data));
    }
    if get2.version != 1 {
        return Ok(format!("FAIL: expected version 1, got {}", get2.version));
    }

    Ok("PASS".to_string())
}
