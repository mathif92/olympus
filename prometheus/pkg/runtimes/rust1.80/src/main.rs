mod handler;

use std::io::Read;

// main adapts the uniform stdin/stdout contract to the user's
// `pub fn handler(event: String) -> String` in src/handler.rs.
// The zip must provide Cargo.toml (package name `handler`) and src/handler.rs,
// and must not ship its own src/main.rs.
fn main() {
    let mut event = String::new();
    if let Err(e) = std::io::stdin().read_to_string(&mut event) {
        eprintln!("error reading event: {e}");
        std::process::exit(1);
    }
    let result = handler::handler(event);
    println!("{result}");
}
