import { Project, SourceFile, SyntaxKind, Node } from 'ts-morph';
import path from 'node:path';

export interface ParsedSymbol {
  name: string;
  kind: 'function' | 'type' | 'interface' | 'class' | 'const' | 'enum';
  line: number;
  exported: boolean;
}

export interface ParsedReference {
  symbolName: string;
  referencedIn: string;
  line: number;
  refKind: 'import' | 'call' | 'type_usage' | 'extends';
}

export interface ParsedImport {
  sourceFile: string;
  targetFile: string;
  importedNames: string[];
}

export interface ParsedFile {
  filePath: string;
  symbols: ParsedSymbol[];
  references: ParsedReference[];
  imports: ParsedImport[];
}

export function createProject(): Project {
  return new Project({
    compilerOptions: { allowJs: true, checkJs: false },
    skipAddingFilesFromTsConfig: true,
  });
}

export function parseFile(filePath: string, project: Project, projectRoot: string): ParsedFile {
  const relativePath = path.relative(projectRoot, filePath);
  let sourceFile: SourceFile;

  try {
    sourceFile = project.addSourceFileAtPath(filePath);
  } catch {
    return { filePath: relativePath, symbols: [], references: [], imports: [] };
  }

  const symbols = extractSymbols(sourceFile, relativePath);
  const references = extractReferences(sourceFile, relativePath, projectRoot);
  const imports = extractImports(sourceFile, relativePath, projectRoot);

  // Remove the source file to avoid memory buildup when parsing many files
  project.removeSourceFile(sourceFile);

  return { filePath: relativePath, symbols, references, imports };
}

function extractSymbols(sourceFile: SourceFile, relativePath: string): ParsedSymbol[] {
  const symbols: ParsedSymbol[] = [];

  // Function declarations
  for (const fn of sourceFile.getFunctions()) {
    const name = fn.getName();
    if (name) {
      symbols.push({
        name,
        kind: 'function',
        line: fn.getStartLineNumber(),
        exported: fn.isExported(),
      });
    }
  }

  // Variable declarations (arrow functions and top-level const/let/var)
  for (const stmt of sourceFile.getVariableStatements()) {
    for (const decl of stmt.getDeclarations()) {
      const name = decl.getName();
      const initializer = decl.getInitializer();
      const isArrowFn = initializer?.getKind() === SyntaxKind.ArrowFunction;
      const isFnExpr = initializer?.getKind() === SyntaxKind.FunctionExpression;

      symbols.push({
        name,
        kind: isArrowFn || isFnExpr ? 'function' : 'const',
        line: decl.getStartLineNumber(),
        exported: stmt.isExported(),
      });
    }
  }

  // Classes
  for (const cls of sourceFile.getClasses()) {
    const name = cls.getName();
    if (name) {
      symbols.push({
        name,
        kind: 'class',
        line: cls.getStartLineNumber(),
        exported: cls.isExported(),
      });
    }
  }

  // Interfaces
  for (const iface of sourceFile.getInterfaces()) {
    symbols.push({
      name: iface.getName(),
      kind: 'interface',
      line: iface.getStartLineNumber(),
      exported: iface.isExported(),
    });
  }

  // Type aliases
  for (const typeAlias of sourceFile.getTypeAliases()) {
    symbols.push({
      name: typeAlias.getName(),
      kind: 'type',
      line: typeAlias.getStartLineNumber(),
      exported: typeAlias.isExported(),
    });
  }

  // Enums
  for (const enumDecl of sourceFile.getEnums()) {
    symbols.push({
      name: enumDecl.getName(),
      kind: 'enum',
      line: enumDecl.getStartLineNumber(),
      exported: enumDecl.isExported(),
    });
  }

  return symbols;
}

function extractReferences(sourceFile: SourceFile, relativePath: string, projectRoot: string): ParsedReference[] {
  const references: ParsedReference[] = [];

  // Import references
  for (const importDecl of sourceFile.getImportDeclarations()) {
    const moduleSpecifier = importDecl.getModuleSpecifierValue();
    const resolvedPath = resolveModulePath(sourceFile, moduleSpecifier, projectRoot);
    if (!resolvedPath) continue;

    // Named imports
    for (const namedImport of importDecl.getNamedImports()) {
      references.push({
        symbolName: namedImport.getName(),
        referencedIn: relativePath,
        line: namedImport.getStartLineNumber(),
        refKind: 'import',
      });
    }

    // Default import
    const defaultImport = importDecl.getDefaultImport();
    if (defaultImport) {
      references.push({
        symbolName: defaultImport.getText(),
        referencedIn: relativePath,
        line: importDecl.getStartLineNumber(),
        refKind: 'import',
      });
    }
  }

  // Call expressions
  for (const callExpr of sourceFile.getDescendantsOfKind(SyntaxKind.CallExpression)) {
    const expr = callExpr.getExpression();
    const name = extractCallName(expr);
    if (name) {
      references.push({
        symbolName: name,
        referencedIn: relativePath,
        line: callExpr.getStartLineNumber(),
        refKind: 'call',
      });
    }
  }

  // Type references (type annotations, generics)
  for (const typeRef of sourceFile.getDescendantsOfKind(SyntaxKind.TypeReference)) {
    const name = typeRef.getTypeName().getText();
    // Skip built-in types
    if (isBuiltinType(name)) continue;
    references.push({
      symbolName: name,
      referencedIn: relativePath,
      line: typeRef.getStartLineNumber(),
      refKind: 'type_usage',
    });
  }

  // Heritage clauses (extends, implements)
  for (const heritageClause of sourceFile.getDescendantsOfKind(SyntaxKind.HeritageClause)) {
    for (const typeNode of heritageClause.getTypeNodes()) {
      const name = typeNode.getExpression().getText();
      references.push({
        symbolName: name,
        referencedIn: relativePath,
        line: typeNode.getStartLineNumber(),
        refKind: 'extends',
      });
    }
  }

  return references;
}

function extractImports(sourceFile: SourceFile, relativePath: string, projectRoot: string): ParsedImport[] {
  const imports: ParsedImport[] = [];

  for (const importDecl of sourceFile.getImportDeclarations()) {
    const moduleSpecifier = importDecl.getModuleSpecifierValue();
    const resolvedPath = resolveModulePath(sourceFile, moduleSpecifier, projectRoot);
    if (!resolvedPath) continue;

    const importedNames: string[] = [];

    // Named imports
    for (const namedImport of importDecl.getNamedImports()) {
      importedNames.push(namedImport.getName());
    }

    // Default import
    const defaultImport = importDecl.getDefaultImport();
    if (defaultImport) {
      importedNames.push(defaultImport.getText());
    }

    // Namespace import
    const namespaceImport = importDecl.getNamespaceImport();
    if (namespaceImport) {
      importedNames.push(`* as ${namespaceImport.getText()}`);
    }

    imports.push({
      sourceFile: relativePath,
      targetFile: resolvedPath,
      importedNames,
    });
  }

  return imports;
}

function resolveModulePath(sourceFile: SourceFile, moduleSpecifier: string, projectRoot: string): string | null {
  // Only resolve relative imports (skip node_modules packages)
  if (!moduleSpecifier.startsWith('.') && !moduleSpecifier.startsWith('/')) {
    return null;
  }

  const sourceDir = path.dirname(sourceFile.getFilePath());
  const resolved = path.resolve(sourceDir, moduleSpecifier);
  const relative = path.relative(projectRoot, resolved);

  // Try common extensions
  const extensions = ['', '.ts', '.tsx', '.js', '.jsx', '/index.ts', '/index.tsx', '/index.js', '/index.jsx'];
  for (const ext of extensions) {
    const candidate = relative + ext;
    // Return the relative path without attempting filesystem checks
    // (the graph builder will match against known files)
    if (ext !== '') {
      return candidate;
    }
  }

  // Return as-is if no extension matched (the import might already have one)
  return relative;
}

function extractCallName(node: Node): string | null {
  if (Node.isIdentifier(node)) {
    return node.getText();
  }
  if (Node.isPropertyAccessExpression(node)) {
    return node.getName();
  }
  return null;
}

function isBuiltinType(name: string): boolean {
  const builtins = new Set([
    'string', 'number', 'boolean', 'void', 'null', 'undefined',
    'never', 'any', 'unknown', 'object', 'symbol', 'bigint',
    'Array', 'Promise', 'Record', 'Partial', 'Required', 'Readonly',
    'Pick', 'Omit', 'Exclude', 'Extract', 'NonNullable', 'ReturnType',
    'Parameters', 'ConstructorParameters', 'InstanceType', 'ThisType',
    'Map', 'Set', 'WeakMap', 'WeakSet', 'Date', 'RegExp', 'Error',
    'Function', 'Object', 'String', 'Number', 'Boolean',
  ]);
  return builtins.has(name);
}
