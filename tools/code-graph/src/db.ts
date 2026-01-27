import Database from 'better-sqlite3';

export interface SymbolRow {
  name: string;
  kind: string;
  file: string;
  line: number;
  exported: boolean;
}

export interface ReferenceRow {
  symbol_id: number;
  referenced_in: string;
  line: number;
  ref_kind: string;
}

export interface ImportRow {
  source_file: string;
  target_file: string;
  imported_names: string[];
}

export interface CallerResult {
  caller: string;
  file: string;
  line: number;
}

export interface CalleeResult {
  callee: string;
  file: string;
  line: number;
}

export interface DependentResult {
  file: string;
  symbols_used: string[];
}

export interface ExportResult {
  name: string;
  kind: string;
  line: number;
}

export interface ImporterResult {
  file: string;
  imported_symbols: string[];
}

export interface TypeUsageResult {
  file: string;
  usage_kind: string;
  line: number;
}

export class CodeGraphDB {
  private db: Database.Database;

  constructor(dbPath: string) {
    this.db = new Database(dbPath);
    this.db.pragma('journal_mode = WAL');
    this.db.pragma('foreign_keys = ON');
  }

  createTables(): void {
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS symbols (
        id INTEGER PRIMARY KEY,
        name TEXT NOT NULL,
        kind TEXT NOT NULL,
        file TEXT NOT NULL,
        line INTEGER NOT NULL,
        exported BOOLEAN DEFAULT 0
      );

      CREATE TABLE IF NOT EXISTS references_ (
        id INTEGER PRIMARY KEY,
        symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
        referenced_in TEXT NOT NULL,
        line INTEGER NOT NULL,
        ref_kind TEXT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS imports (
        id INTEGER PRIMARY KEY,
        source_file TEXT NOT NULL,
        target_file TEXT NOT NULL,
        imported_names TEXT
      );

      CREATE TABLE IF NOT EXISTS metadata (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL
      );

      CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
      CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file);
      CREATE INDEX IF NOT EXISTS idx_references_symbol ON references_(symbol_id);
      CREATE INDEX IF NOT EXISTS idx_references_referenced_in ON references_(referenced_in);
      CREATE INDEX IF NOT EXISTS idx_imports_source ON imports(source_file);
      CREATE INDEX IF NOT EXISTS idx_imports_target ON imports(target_file);
    `);
  }

  close(): void {
    this.db.close();
  }

  // --- Write operations ---

  insertSymbol(symbol: SymbolRow): number {
    const stmt = this.db.prepare(
      'INSERT INTO symbols (name, kind, file, line, exported) VALUES (?, ?, ?, ?, ?)'
    );
    const result = stmt.run(
      symbol.name,
      symbol.kind,
      symbol.file,
      symbol.line,
      symbol.exported ? 1 : 0
    );
    return Number(result.lastInsertRowid);
  }

  insertReference(ref: ReferenceRow): number {
    const stmt = this.db.prepare(
      'INSERT INTO references_ (symbol_id, referenced_in, line, ref_kind) VALUES (?, ?, ?, ?)'
    );
    const result = stmt.run(ref.symbol_id, ref.referenced_in, ref.line, ref.ref_kind);
    return Number(result.lastInsertRowid);
  }

  insertImport(imp: ImportRow): number {
    const stmt = this.db.prepare(
      'INSERT INTO imports (source_file, target_file, imported_names) VALUES (?, ?, ?)'
    );
    const result = stmt.run(
      imp.source_file,
      imp.target_file,
      JSON.stringify(imp.imported_names)
    );
    return Number(result.lastInsertRowid);
  }

  deleteFileEntries(filePath: string): void {
    const deleteInTransaction = this.db.transaction((fp: string) => {
      // Delete references that point to symbols in this file
      this.db.prepare(
        'DELETE FROM references_ WHERE symbol_id IN (SELECT id FROM symbols WHERE file = ?)'
      ).run(fp);
      // Delete references that occur in this file
      this.db.prepare('DELETE FROM references_ WHERE referenced_in = ?').run(fp);
      // Delete symbols defined in this file
      this.db.prepare('DELETE FROM symbols WHERE file = ?').run(fp);
      // Delete imports from/to this file
      this.db.prepare('DELETE FROM imports WHERE source_file = ? OR target_file = ?').run(fp, fp);
    });
    deleteInTransaction(filePath);
  }

  // --- Read operations ---

  getCallers(symbolName: string, filePath?: string, limit = 20): CallerResult[] {
    let query: string;
    let params: (string | number)[];

    if (filePath) {
      query = `
        SELECT r.referenced_in AS file, r.line,
               COALESCE(caller_sym.name, '(top-level)') AS caller
        FROM references_ r
        JOIN symbols s ON r.symbol_id = s.id
        LEFT JOIN symbols caller_sym ON caller_sym.file = r.referenced_in
          AND caller_sym.line <= r.line
          AND caller_sym.kind IN ('function', 'class')
        WHERE s.name = ? AND s.file = ? AND r.ref_kind = 'call'
        GROUP BY r.id
        ORDER BY r.referenced_in, r.line
        LIMIT ?
      `;
      params = [symbolName, filePath, limit];
    } else {
      query = `
        SELECT r.referenced_in AS file, r.line,
               COALESCE(caller_sym.name, '(top-level)') AS caller
        FROM references_ r
        JOIN symbols s ON r.symbol_id = s.id
        LEFT JOIN symbols caller_sym ON caller_sym.file = r.referenced_in
          AND caller_sym.line <= r.line
          AND caller_sym.kind IN ('function', 'class')
        WHERE s.name = ? AND r.ref_kind = 'call'
        GROUP BY r.id
        ORDER BY r.referenced_in, r.line
        LIMIT ?
      `;
      params = [symbolName, limit];
    }

    return this.db.prepare(query).all(...params) as CallerResult[];
  }

  getCallees(symbolName: string, filePath?: string, limit = 20): CalleeResult[] {
    // Find references made FROM the file where symbolName is defined
    let query: string;
    let params: (string | number)[];

    if (filePath) {
      query = `
        SELECT DISTINCT s_target.name AS callee, s_target.file AS file, r.line
        FROM symbols s_source
        JOIN references_ r ON r.referenced_in = s_source.file AND r.ref_kind = 'call'
        JOIN symbols s_target ON r.symbol_id = s_target.id
        WHERE s_source.name = ? AND s_source.file = ?
        ORDER BY r.line
        LIMIT ?
      `;
      params = [symbolName, filePath, limit];
    } else {
      query = `
        SELECT DISTINCT s_target.name AS callee, s_target.file AS file, r.line
        FROM symbols s_source
        JOIN references_ r ON r.referenced_in = s_source.file AND r.ref_kind = 'call'
        JOIN symbols s_target ON r.symbol_id = s_target.id
        WHERE s_source.name = ?
        ORDER BY r.line
        LIMIT ?
      `;
      params = [symbolName, limit];
    }

    return this.db.prepare(query).all(...params) as CalleeResult[];
  }

  getDependents(filePath: string, limit = 20): DependentResult[] {
    const rows = this.db.prepare(`
      SELECT source_file AS file, imported_names
      FROM imports
      WHERE target_file = ?
      LIMIT ?
    `).all(filePath, limit) as Array<{ file: string; imported_names: string }>;

    return rows.map(row => ({
      file: row.file,
      symbols_used: JSON.parse(row.imported_names || '[]') as string[],
    }));
  }

  getExports(filePath: string, limit = 20): ExportResult[] {
    return this.db.prepare(`
      SELECT name, kind, line
      FROM symbols
      WHERE file = ? AND exported = 1
      ORDER BY line
      LIMIT ?
    `).all(filePath, limit) as ExportResult[];
  }

  getImporters(filePath: string, limit = 20): ImporterResult[] {
    const rows = this.db.prepare(`
      SELECT source_file AS file, imported_names
      FROM imports
      WHERE target_file = ?
      LIMIT ?
    `).all(filePath, limit) as Array<{ file: string; imported_names: string }>;

    return rows.map(row => ({
      file: row.file,
      imported_symbols: JSON.parse(row.imported_names || '[]') as string[],
    }));
  }

  getTypeUsages(typeName: string, limit = 20): TypeUsageResult[] {
    return this.db.prepare(`
      SELECT r.referenced_in AS file, r.ref_kind AS usage_kind, r.line
      FROM references_ r
      JOIN symbols s ON r.symbol_id = s.id
      WHERE s.name = ? AND r.ref_kind IN ('type_usage', 'extends', 'import')
      ORDER BY r.referenced_in, r.line
      LIMIT ?
    `).all(typeName, limit) as TypeUsageResult[];
  }

  getSymbolDefinition(symbolName: string): { file: string; line: number; kind: string } | null {
    const row = this.db.prepare(`
      SELECT file, line, kind FROM symbols WHERE name = ? LIMIT 1
    `).get(symbolName) as { file: string; line: number; kind: string } | undefined;
    return row ?? null;
  }

  getSymbolId(symbolName: string, filePath?: string): number | null {
    let row: { id: number } | undefined;
    if (filePath) {
      row = this.db.prepare(
        'SELECT id FROM symbols WHERE name = ? AND file = ? LIMIT 1'
      ).get(symbolName, filePath) as { id: number } | undefined;
    } else {
      row = this.db.prepare(
        'SELECT id FROM symbols WHERE name = ? LIMIT 1'
      ).get(symbolName) as { id: number } | undefined;
    }
    return row?.id ?? null;
  }

  getIndexedFiles(): string[] {
    const rows = this.db.prepare('SELECT DISTINCT file FROM symbols ORDER BY file').all() as Array<{ file: string }>;
    return rows.map(r => r.file);
  }

  // --- Metadata ---

  getMetadata(key: string): string | null {
    const row = this.db.prepare('SELECT value FROM metadata WHERE key = ?').get(key) as { value: string } | undefined;
    return row?.value ?? null;
  }

  setMetadata(key: string, value: string): void {
    this.db.prepare(
      'INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)'
    ).run(key, value);
  }
}
