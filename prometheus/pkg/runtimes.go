package pkg

import "embed"

// Runtime describes one supported language runtime and the restriction the
// uploaded code must satisfy in order to run: a fixed entrypoint file (with a
// fixed handler signature) that the executor's launcher scaffold wires up.
//
// The "run function that receives parameters" contract is identical for every
// runtime: the invocation event is delivered as JSON on the handler's stdin
// and the handler's JSON result is written to stdout. Each runtime only
// dictates how that handler is exposed in its own language.
type Runtime struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	Handler       string   `json:"handler"`
	HandlerFile   string   `json:"handler_file"`
	HandlerFunc   string   `json:"handler_func"`
	RequiredFiles []string `json:"required_files"`
	Description   string   `json:"description"`
}

var runtimes = []Runtime{
	{
		ID: "python3.12", Name: "Python 3.12", Image: "python:3.12-alpine",
		Handler: "handler.handler", HandlerFile: "handler.py", HandlerFunc: "def handler(event)",
		RequiredFiles: []string{"handler.py"},
		Description:   "Python handler receives the event dict and returns any JSON-serialisable value.",
	},
	{
		ID: "nodejs20", Name: "Node.js 20 (JavaScript)", Image: "node:20-alpine",
		Handler: "handler.handler", HandlerFile: "handler.js", HandlerFunc: "exports.handler = async (event) => ...",
		RequiredFiles: []string{"handler.js"},
		Description:   "CommonJS handler module; `exports.handler(event)` may be async and returns JSON-serialisable data.",
	},
	{
		ID: "typescript5", Name: "TypeScript 5", Image: "node:20-alpine",
		Handler: "handler.handler", HandlerFile: "handler.ts", HandlerFunc: "export function handler(event)",
		RequiredFiles: []string{"handler.ts"},
		Description:   "TypeScript compiled to JavaScript at build time; `export function handler(event)` may be async.",
	},
	{
		ID: "java21", Name: "Java 21", Image: "eclipse-temurin:21-jdk-alpine",
		Handler: "Handler.handler", HandlerFile: "Handler.java", HandlerFunc: "public static String handler(String event)",
		RequiredFiles: []string{"Handler.java"},
		Description:   "Default-package class `Handler` with a static `handler(String event)` returning a JSON string.",
	},
	{
		ID: "go1.25", Name: "Go 1.25", Image: "golang:1.25-alpine",
		Handler: "main.Handler", HandlerFile: "handler.go", HandlerFunc: "func Handler(event string) (string, error)",
		RequiredFiles: []string{"handler.go"},
		Description:   "`package main` file with `func Handler(event string) (string, error)`; compiled to a static binary.",
	},
	{
		ID: "rust1.80", Name: "Rust 1.80", Image: "rust:1.80-alpine",
		Handler: "handler::handler", HandlerFile: "src/handler.rs", HandlerFunc: "pub fn handler(event: String) -> String",
		RequiredFiles: []string{"Cargo.toml", "src/handler.rs"},
		Description:   "Crate named `handler` (Cargo.toml package name) with `src/handler.rs` exposing `pub fn handler(String) -> String`.",
	},
	{
		ID: "dotnet9", Name: "C# (.NET 9)", Image: "mcr.microsoft.com/dotnet/sdk:9.0",
		Handler: "Program.Handler", HandlerFile: "Program.cs", HandlerFunc: "static string Handler(string eventJson)",
		RequiredFiles: []string{"Program.cs"},
		Description:   "`Program.cs` with a static `Handler(string eventJson)` returning a JSON string; no .csproj in the zip.",
	},
	{
		ID: "ruby3.3", Name: "Ruby 3.3", Image: "ruby:3.3-alpine",
		Handler: "handler", HandlerFile: "handler.rb", HandlerFunc: "def handler(event)",
		RequiredFiles: []string{"handler.rb"},
		Description:   "`handler.rb` defines a top-level `def handler(event)` returning JSON-serialisable data.",
	},
}

// runtimesFS embeds the per-runtime Dockerfile + launcher scaffolding so the
// executor can materialise a build context without depending on the working
// directory or the on-disk layout at runtime.
//
//go:embed runtimes/*
var runtimesFS embed.FS

// ListRuntimes returns all supported runtimes.
func ListRuntimes() []Runtime {
	out := make([]Runtime, len(runtimes))
	copy(out, runtimes)
	return out
}

// GetRuntime returns the runtime with the given id.
func GetRuntime(id string) (Runtime, bool) {
	for _, r := range runtimes {
		if r.ID == id {
			return r, true
		}
	}
	return Runtime{}, false
}
