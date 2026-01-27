import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';
import type { CallToolResult } from '@modelcontextprotocol/sdk/types.js';
import { CodeGraphDB } from './db.js';
import { buildGraph, updateFiles } from './graph.js';

export function createServer(db: CodeGraphDB, projectRoot: string): Server {
  const server = new Server(
    { name: 'code-graph', version: '0.1.0' },
    { capabilities: { tools: { listChanged: false } } }
  );

  server.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: [
      {
        name: 'get_callers',
        description: `WHO calls this function/method? Direction: callers -> function.

Use this to find all call sites for a given function. Useful when you need to understand
how widely used a function is before modifying its signature.

Example input: { "symbol_name": "validateUser", "file_path": "src/auth/validate.ts" }
Example output: {
  "callers": [
    { "caller": "loginHandler", "file": "src/routes/auth.ts", "line": 45 },
    { "caller": "signupHandler", "file": "src/routes/auth.ts", "line": 89 }
  ]
}

file_path is optional -- if omitted, searches all files for the symbol name.
All query tools are safe to retry (idempotent).
See also: get_dependents for broader transitive impact analysis.`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            symbol_name: { type: 'string', description: 'Function or method name to find callers of' },
            file_path: { type: 'string', description: 'Optional: file where the function is defined (relative to project root)' },
            limit: { type: 'number', description: 'Max results to return (default 20)' },
          },
          required: ['symbol_name'],
        },
      },
      {
        name: 'get_callees',
        description: `WHAT does this function call? Direction: function -> callees.

Use this to understand what a function depends on internally. Useful for understanding
the implementation details and downstream effects of a function.

Example input: { "symbol_name": "processOrder" }
Example output: {
  "callees": [
    { "callee": "validateOrder", "file": "src/orders/validate.ts", "line": 12 },
    { "callee": "chargePayment", "file": "src/payments/charge.ts", "line": 34 }
  ]
}

All query tools are safe to retry (idempotent).
See also: get_callers for the reverse direction.`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            symbol_name: { type: 'string', description: 'Function or method name to find callees of' },
            file_path: { type: 'string', description: 'Optional: file where the function is defined' },
            limit: { type: 'number', description: 'Max results to return (default 20)' },
          },
          required: ['symbol_name'],
        },
      },
      {
        name: 'get_dependents',
        description: `WHAT breaks if this file changes? Broader than get_callers -- includes transitive deps.

Use this to understand the blast radius of changing a file. Returns all files that
import from or depend on the given file, directly or transitively.

Example input: { "file_path": "src/types/user.ts" }
Example output: {
  "dependents": [
    { "file": "src/auth/validate.ts", "symbols_used": ["User", "UserRole"] },
    { "file": "src/routes/profile.ts", "symbols_used": ["User"] }
  ]
}

All query tools are safe to retry (idempotent).
When to use: Before modifying exports of a file, to know what will break.
See also: get_callers for function-level (narrower) analysis.`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            file_path: { type: 'string', description: 'File path relative to project root' },
            limit: { type: 'number', description: 'Max results to return (default 20)' },
          },
          required: ['file_path'],
        },
      },
      {
        name: 'get_exports',
        description: `What does this file export? Returns all exported functions, types, constants, classes.

Use this to quickly understand a file's public API without reading the full file.

Example input: { "file_path": "src/utils/string.ts" }
Example output: {
  "exports": [
    { "name": "capitalize", "kind": "function", "line": 5 },
    { "name": "StringOptions", "kind": "interface", "line": 1 },
    { "name": "MAX_LENGTH", "kind": "const", "line": 15 }
  ]
}

All query tools are safe to retry (idempotent).`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            file_path: { type: 'string', description: 'File path relative to project root' },
            limit: { type: 'number', description: 'Max results to return (default 20)' },
          },
          required: ['file_path'],
        },
      },
      {
        name: 'get_importers',
        description: `Who imports from this file? Returns all files that have import statements pointing to this file.

Use this to understand how a module is consumed across the codebase.

Example input: { "file_path": "src/utils/date.ts" }
Example output: {
  "importers": [
    { "file": "src/components/Calendar.tsx", "imported_symbols": ["formatDate", "parseDate"] },
    { "file": "src/api/events.ts", "imported_symbols": ["DateRange"] }
  ]
}

All query tools are safe to retry (idempotent).
See also: get_dependents for transitive dependency analysis (broader).`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            file_path: { type: 'string', description: 'File path relative to project root' },
            limit: { type: 'number', description: 'Max results to return (default 20)' },
          },
          required: ['file_path'],
        },
      },
      {
        name: 'get_type_usages',
        description: `Where is this type/interface defined and used? Tracks type annotations, generics, extends, implements.

Use this to understand the reach of a type before modifying it.

Example input: { "type_name": "UserRole" }
Example output: {
  "definition": { "file": "src/types/user.ts", "line": 12, "kind": "type" },
  "usages": [
    { "file": "src/auth/validate.ts", "usage_kind": "type_usage", "line": 8 },
    { "file": "src/middleware/rbac.ts", "usage_kind": "type_usage", "line": 23 },
    { "file": "src/models/user.ts", "usage_kind": "extends", "line": 5 }
  ]
}

All query tools are safe to retry (idempotent).`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            type_name: { type: 'string', description: 'Type or interface name to find usages of' },
            limit: { type: 'number', description: 'Max results to return (default 20)' },
          },
          required: ['type_name'],
        },
      },
      {
        name: 'understand_file',
        description: `Get a complete understanding of a file in ONE call. Returns exports, importers, callers of
exported functions, and types defined -- everything you need to understand a file's role.

This is a composite tool that combines get_exports + get_importers + get_callers + get_type_usages
into a single response. Use this instead of making 4 separate calls.

Example input: { "file_path": "src/auth/validate.ts" }
Example output: {
  "file": "src/auth/validate.ts",
  "exports": [
    { "name": "validateUser", "kind": "function", "line": 15 },
    { "name": "ValidationResult", "kind": "type", "line": 3 }
  ],
  "importers": [
    { "file": "src/routes/auth.ts", "imported_symbols": ["validateUser"] }
  ],
  "callers": {
    "validateUser": [
      { "caller": "loginHandler", "file": "src/routes/auth.ts", "line": 45 }
    ]
  },
  "types": {
    "ValidationResult": [
      { "file": "src/routes/auth.ts", "usage_kind": "type_usage", "line": 12 }
    ]
  }
}

All query tools are safe to retry (idempotent).`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            file_path: { type: 'string', description: 'File path relative to project root' },
          },
          required: ['file_path'],
        },
      },
      {
        name: 'analyze_impact',
        description: `Full blast radius analysis: what breaks if this file/symbol changes?

Returns direct dependents, transitive dependents (dependents of dependents), and affected test files.
Use this during UNDERSTAND phase to scope the impact of a planned change.

Example input: { "file_path": "src/types/user.ts", "symbol_name": "User" }
Example output: {
  "direct_dependents": [
    { "file": "src/auth/validate.ts", "symbols_used": ["User"] },
    { "file": "src/routes/profile.ts", "symbols_used": ["User", "UserProfile"] }
  ],
  "transitive_dependents": [
    { "file": "src/routes/auth.ts", "via": "src/auth/validate.ts" },
    { "file": "src/middleware/auth.ts", "via": "src/auth/validate.ts" }
  ],
  "affected_tests": [
    "src/auth/__tests__/validate.test.ts",
    "src/routes/__tests__/profile.test.ts"
  ]
}

symbol_name is optional -- if omitted, analyzes impact of the entire file.
All query tools are safe to retry (idempotent).`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            file_path: { type: 'string', description: 'File path relative to project root' },
            symbol_name: { type: 'string', description: 'Optional: specific symbol to analyze (if omitted, analyzes entire file)' },
            limit: { type: 'number', description: 'Max results per category (default 20)' },
          },
          required: ['file_path'],
        },
      },
      {
        name: 'reindex_files',
        description: `Incrementally reparse only the specified files (~50ms each).
Called by the Berth Go binary between beads after git commits. NOT available to executor Claude.

Example input: { "file_paths": ["src/auth/validate.ts", "src/types/user.ts"] }
Example output: { "reindexed_count": 2 }`,
        inputSchema: {
          type: 'object' as const,
          properties: {
            file_paths: {
              type: 'array',
              items: { type: 'string' },
              description: 'List of file paths (relative to project root) to reindex',
            },
          },
          required: ['file_paths'],
        },
      },
      {
        name: 'reindex',
        description: `Force full reindex of entire codebase. Called by Berth Go binary only.
Repopulates the entire knowledge graph from scratch.

Example input: {}
Example output: { "file_count": 142, "symbol_count": 1893, "reference_count": 4521 }`,
        inputSchema: {
          type: 'object' as const,
          properties: {},
          required: [],
        },
      },
    ],
  }));

  server.setRequestHandler(CallToolRequestSchema, async (request): Promise<CallToolResult> => {
    const { name, arguments: args } = request.params;

    switch (name) {
      case 'get_callers': {
        const symbolName = args?.symbol_name as string;
        const filePath = args?.file_path as string | undefined;
        const limit = (args?.limit as number) ?? 20;
        const callers = db.getCallers(symbolName, filePath, limit);
        return jsonResult({ callers });
      }

      case 'get_callees': {
        const symbolName = args?.symbol_name as string;
        const filePath = args?.file_path as string | undefined;
        const limit = (args?.limit as number) ?? 20;
        const callees = db.getCallees(symbolName, filePath, limit);
        return jsonResult({ callees });
      }

      case 'get_dependents': {
        const filePath = args?.file_path as string;
        const limit = (args?.limit as number) ?? 20;
        const dependents = db.getDependents(filePath, limit);
        return jsonResult({ dependents });
      }

      case 'get_exports': {
        const filePath = args?.file_path as string;
        const limit = (args?.limit as number) ?? 20;
        const exports = db.getExports(filePath, limit);
        return jsonResult({ exports });
      }

      case 'get_importers': {
        const filePath = args?.file_path as string;
        const limit = (args?.limit as number) ?? 20;
        const importers = db.getImporters(filePath, limit);
        return jsonResult({ importers });
      }

      case 'get_type_usages': {
        const typeName = args?.type_name as string;
        const limit = (args?.limit as number) ?? 20;
        const definition = db.getSymbolDefinition(typeName);
        const usages = db.getTypeUsages(typeName, limit);
        return jsonResult({ definition, usages });
      }

      case 'understand_file': {
        const filePath = args?.file_path as string;
        const exports = db.getExports(filePath);
        const importers = db.getImporters(filePath);

        // Get callers for each exported function
        const callers: Record<string, Array<{ caller: string; file: string; line: number }>> = {};
        for (const exp of exports) {
          if (exp.kind === 'function') {
            const fnCallers = db.getCallers(exp.name, filePath);
            if (fnCallers.length > 0) {
              callers[exp.name] = fnCallers;
            }
          }
        }

        // Get type usages for each exported type/interface
        const types: Record<string, Array<{ file: string; usage_kind: string; line: number }>> = {};
        for (const exp of exports) {
          if (exp.kind === 'type' || exp.kind === 'interface') {
            const usages = db.getTypeUsages(exp.name);
            if (usages.length > 0) {
              types[exp.name] = usages;
            }
          }
        }

        return jsonResult({ file: filePath, exports, importers, callers, types });
      }

      case 'analyze_impact': {
        const filePath = args?.file_path as string;
        const limit = (args?.limit as number) ?? 20;

        // Direct dependents: files that import from this file
        const directDependents = db.getDependents(filePath, limit);

        // Transitive dependents: files that import from direct dependents
        const transitiveDependents: Array<{ file: string; via: string }> = [];
        const seen = new Set<string>([filePath]);

        for (const dep of directDependents) {
          seen.add(dep.file);
        }

        for (const dep of directDependents) {
          const secondLevel = db.getDependents(dep.file, limit);
          for (const td of secondLevel) {
            if (!seen.has(td.file)) {
              seen.add(td.file);
              transitiveDependents.push({ file: td.file, via: dep.file });
            }
          }
        }

        // Affected tests: filter all dependents for test file patterns
        const testPattern = /\.(test|spec)\.(ts|tsx|js|jsx)$|__tests__\//;
        const allFiles = [
          ...directDependents.map(d => d.file),
          ...transitiveDependents.map(d => d.file),
        ];
        const affectedTests = [...new Set(allFiles.filter(f => testPattern.test(f)))];

        return jsonResult({
          direct_dependents: directDependents,
          transitive_dependents: transitiveDependents,
          affected_tests: affectedTests,
        });
      }

      case 'reindex_files': {
        const filePaths = args?.file_paths as string[];
        const result = updateFiles(filePaths, projectRoot, db);
        return jsonResult(result);
      }

      case 'reindex': {
        const result = buildGraph(projectRoot, db);
        return jsonResult(result);
      }

      default:
        return {
          content: [{ type: 'text', text: `Unknown tool: ${name}` }],
          isError: true,
        };
    }
  });

  return server;
}

function jsonResult(data: unknown): CallToolResult {
  return {
    content: [{ type: 'text', text: JSON.stringify(data, null, 2) }],
  };
}
