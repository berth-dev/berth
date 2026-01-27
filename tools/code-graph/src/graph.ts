import path from 'node:path';
import { glob } from 'glob';
import { CodeGraphDB } from './db.js';
import { createProject, parseFile } from './parser.js';

export interface BuildResult {
  file_count: number;
  symbol_count: number;
  reference_count: number;
}

export interface UpdateResult {
  reindexed_count: number;
}

const SOURCE_GLOB = '**/*.{ts,tsx,js,jsx}';
const IGNORE_PATTERNS = ['**/node_modules/**', '**/dist/**', '**/.berth/**', '**/coverage/**', '**/.git/**'];

export function buildGraph(projectRoot: string, db: CodeGraphDB): BuildResult {
  const files = glob.sync(SOURCE_GLOB, {
    cwd: projectRoot,
    absolute: true,
    ignore: IGNORE_PATTERNS,
  });

  const project = createProject();
  let symbolCount = 0;
  let referenceCount = 0;

  for (const filePath of files) {
    const parsed = parseFile(filePath, project, projectRoot);
    const { symbols, references, imports } = indexParsedFile(parsed, db);
    symbolCount += symbols;
    referenceCount += references;
  }

  db.setMetadata('last_index_time', new Date().toISOString());

  return {
    file_count: files.length,
    symbol_count: symbolCount,
    reference_count: referenceCount,
  };
}

export function updateFiles(changedFiles: string[], projectRoot: string, db: CodeGraphDB): UpdateResult {
  const project = createProject();

  for (const file of changedFiles) {
    const absolutePath = path.isAbsolute(file) ? file : path.join(projectRoot, file);
    const relativePath = path.relative(projectRoot, absolutePath);

    // Remove old entries
    db.deleteFileEntries(relativePath);

    // Reparse and reinsert
    const parsed = parseFile(absolutePath, project, projectRoot);
    indexParsedFile(parsed, db);
  }

  db.setMetadata('last_index_time', new Date().toISOString());

  return { reindexed_count: changedFiles.length };
}

export function removeFiles(deletedFiles: string[], projectRoot: string, db: CodeGraphDB): void {
  for (const file of deletedFiles) {
    const relativePath = path.isAbsolute(file) ? path.relative(projectRoot, file) : file;
    db.deleteFileEntries(relativePath);
  }
}

interface IndexCounts {
  symbols: number;
  references: number;
  imports: number;
}

function indexParsedFile(
  parsed: ReturnType<typeof parseFile>,
  db: CodeGraphDB
): IndexCounts {
  let symbolCount = 0;
  let referenceCount = 0;
  let importCount = 0;

  // Build a map of symbolName -> symbolId for linking references
  const symbolIds = new Map<string, number>();

  for (const sym of parsed.symbols) {
    const id = db.insertSymbol({
      name: sym.name,
      kind: sym.kind,
      file: parsed.filePath,
      line: sym.line,
      exported: sym.exported,
    });
    symbolIds.set(sym.name, id);
    symbolCount++;
  }

  for (const ref of parsed.references) {
    // Try to find the symbol ID for this reference
    const symbolId = symbolIds.get(ref.symbolName);
    if (symbolId !== undefined) {
      db.insertReference({
        symbol_id: symbolId,
        referenced_in: ref.referencedIn,
        line: ref.line,
        ref_kind: ref.refKind,
      });
      referenceCount++;
    }
    // If we can't find the symbol locally, look up globally for cross-file references
    else {
      const globalId = db.getSymbolId(ref.symbolName);
      if (globalId !== null) {
        db.insertReference({
          symbol_id: globalId,
          referenced_in: ref.referencedIn,
          line: ref.line,
          ref_kind: ref.refKind,
        });
        referenceCount++;
      }
    }
  }

  for (const imp of parsed.imports) {
    db.insertImport({
      source_file: imp.sourceFile,
      target_file: imp.targetFile,
      imported_names: imp.importedNames,
    });
    importCount++;
  }

  return { symbols: symbolCount, references: referenceCount, imports: importCount };
}
