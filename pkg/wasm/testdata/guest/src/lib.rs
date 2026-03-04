use extism_pdk::*;

#[host_fn]
extern "ExtismHost" {
    fn host_echo(input: String) -> String;
}

/// Exported function that reads input, calls the host_echo host function,
/// and returns the host's response. This proves the full round-trip:
/// Go host → WASM guest → host function → WASM guest → Go host.
#[plugin_fn]
pub fn run(input: String) -> FnResult<String> {
    let response = unsafe { host_echo(input)? };
    Ok(response)
}
