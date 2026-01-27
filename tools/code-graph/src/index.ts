import path from 'node:path';
import process from 'node:process';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CodeGraphDB } from './db.js';
import { createServer } from './server.js';

const projectRoot = process.env.BERTH_PROJECT_ROOT || process.cwd();
const dbPath = path.join(projectRoot, 'tools', 'code-graph', 'code-graph.db');

const db = new CodeGraphDB(dbPath);
db.createTables();

const server = createServer(db, projectRoot);
const transport = new StdioServerTransport();

function shutdown() {
  db.close();
  process.exit(0);
}

process.on('SIGTERM', shutdown);
process.on('SIGINT', shutdown);

await server.connect(transport);
