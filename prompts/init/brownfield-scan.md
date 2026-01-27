You are analyzing an existing codebase for Berth initialization.

Scan the project thoroughly and produce a structured analysis. Examine the directory layout, configuration files, source code, and any documentation present.

Focus on:
- Overall architecture (monolith, microservices, modular monolith, etc.)
- Primary language(s) and framework(s) in use
- Key patterns (dependency injection, repository pattern, MVC, event-driven, etc.)
- Coding conventions (naming, file organization, error handling style)
- Entry points (main files, CLI commands, HTTP handlers, event listeners)
- Test strategy (unit tests, integration tests, e2e, test runner, coverage)
- Build and deployment configuration
- Dependency management approach

Output format (JSON):
{
  "architecture": {
    "style": "Description of the architecture style",
    "layers": ["List of architectural layers or modules"],
    "keyDirectories": {"path": "purpose"}
  },
  "patterns": {
    "design": ["Design patterns observed"],
    "antiPatterns": ["Any anti-patterns noticed"],
    "idioms": ["Language-specific idioms in use"]
  },
  "conventions": {
    "naming": "Naming convention description",
    "fileOrganization": "How files and directories are structured",
    "errorHandling": "Error handling approach",
    "logging": "Logging approach"
  },
  "entryPoints": [
    {"file": "path/to/file", "type": "http|cli|event|cron", "description": "What it does"}
  ],
  "testStrategy": {
    "framework": "Test framework in use",
    "patterns": ["Test patterns observed"],
    "coverage": "Coverage approach or tools",
    "locations": ["Where tests live"]
  }
}
