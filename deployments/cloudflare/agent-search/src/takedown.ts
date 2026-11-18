import { create, fromJson, toJson } from '@bufbuild/protobuf';
import { TakedownApplyRequestSchema, TakedownPlanRequestSchema, TakedownReceiptSchema } from '../gen/emomo/v1/import_pb.js';
import { ID, validateStorageKey } from './metadata.js';
import { SHA256 } from '../../../../cli/src/import-metadata.js';
import { ImportError, json, jsonBody, hex, OPTIONS, HASH_SQL } from './maintenance-http.js';

type Saved = { sha256: string; targets: string; taken_down_at: string };
type Target = { id: string; storage_key: string };
const KEY_SQL = "json_extract(meme_json, '$.storage_key')";
const targetsQuery = `SELECT id, ${KEY_SQL} AS storage_key FROM memes WHERE ${HASH_SQL} = ? ORDER BY id LIMIT 101`;
const previewChanged = () => new ImportError(409, 'TAKEDOWN_PREVIEW_CHANGED', 'The affected records changed. Review a fresh takedown plan.');
const notFound = () => new ImportError(404, 'TAKEDOWN_NOT_FOUND', 'No production image matches this selector.');

export async function isTakenDown(db: D1Database, sha256: string): Promise<boolean> {
  return (await db.prepare('SELECT 1 FROM library_takedowns WHERE sha256 = ?').bind(sha256).first()) !== null;
}

async function saved(db: D1Database, sha256: string) {
  return db.prepare('SELECT sha256, targets, taken_down_at FROM library_takedowns WHERE sha256 = ?').bind(sha256).first<Saved>();
}

async function targetHash(db: D1Database, id: string): Promise<string> {
  const active = await db.prepare(`SELECT ${HASH_SQL} AS sha256 FROM memes WHERE id = ?`).bind(id).first<{ sha256: string | null }>();
  if (active) {
    if (!active.sha256 || !SHA256.test(active.sha256)) {
      throw new ImportError(409, 'TAKEDOWN_HASH_UNAVAILABLE', 'This historical image needs a verified SHA-256 before it can be taken down.');
    }
    return active.sha256;
  }
  const match = await db.prepare(`SELECT sha256 FROM library_takedowns
    WHERE EXISTS (SELECT 1 FROM json_each(targets) WHERE json_extract(value, '$[0]') = ?) LIMIT 2`).bind(id).all<{ sha256: string }>();
  if (!match.results.length) throw notFound();
  if (match.results.length > 1) throw new ImportError(409, 'TAKEDOWN_ID_AMBIGUOUS', 'Use the exact SHA-256 for this historical ID.');
  return match.results[0].sha256;
}

async function receipt(row: Saved) {
  const targets = JSON.parse(row.targets) as [string, string][];
  const confirmation = hex(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(JSON.stringify([row.sha256, targets]))));
  return {
    sha256: row.sha256, confirmation, takenDownAt: row.taken_down_at,
    targets: targets.map(([id, storageKey]) => ({ id, storageKey })),
  };
}

async function current(db: D1Database, sha256: string): Promise<Saved | null> {
  const existing = await saved(db, sha256);
  if (existing) return existing;
  const { results } = await db.prepare(targetsQuery).bind(sha256).all<Target>();
  if (!results.length) return saved(db, sha256);
  if (results.length > 100) throw new ImportError(409, 'TAKEDOWN_TOO_MANY_TARGETS', 'This image has more than 100 index records.');
  return { sha256, targets: JSON.stringify(results.map(row => [row.id, validateStorageKey(row.storage_key)])), taken_down_at: '' };
}

export async function takedown(request: Request, db: D1Database, apply: boolean) {
  let sha256: string;
  let confirmation = '';
  if (apply) {
    const input = await jsonBody(request, TakedownApplyRequestSchema);
    ({ sha256, confirmation } = input);
    if (!SHA256.test(sha256) || !SHA256.test(confirmation)) {
      throw new ImportError(400, 'INVALID_TAKEDOWN', 'Provide a SHA-256 and its preview confirmation.');
    }
  } else {
    const { selector } = await jsonBody(request, TakedownPlanRequestSchema);
    if (selector.case === 'id' && ID.test(selector.value)) sha256 = await targetHash(db, selector.value);
    else if (selector.case === 'sha256' && SHA256.test(selector.value)) sha256 = selector.value;
    else throw new ImportError(400, 'INVALID_TAKEDOWN', 'Provide exactly one production image ID or SHA-256.');
  }
  const prepared = await current(db, sha256);
  if (!prepared) throw apply ? previewChanged() : notFound();
  let result = await receipt(prepared);
  if (apply) {
    if (result.confirmation !== confirmation) throw previewChanged();
    if (!prepared.taken_down_at) {
      // The SQL compares the exact preview again under the writer lock.
      // The INSERT trigger removes the catalog rows and their FTS entries atomically.
      const inserted = await db.prepare(`INSERT INTO library_takedowns (sha256, targets)
        SELECT ?, ? WHERE ? = (
          SELECT json_group_array(json_array(id, storage_key)) FROM (${targetsQuery})
        )
        ON CONFLICT(sha256) DO NOTHING
        RETURNING sha256, targets, taken_down_at`).bind(sha256, prepared.targets, prepared.targets, sha256).first<Saved>();
      const committed = inserted ?? await saved(db, sha256);
      if (!committed) throw previewChanged();
      result = await receipt(committed);
      if (result.confirmation !== confirmation) throw previewChanged();
    }
  }
  return json(toJson(TakedownReceiptSchema, create(TakedownReceiptSchema, result), OPTIONS));
}
