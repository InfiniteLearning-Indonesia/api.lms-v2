import { readFile } from 'node:fs/promises';
import type { DataSource, EntityManager } from 'typeorm';
import { runSeed } from './seed-data-source.js';

export interface NimRow {
  nama: string;
  nim: string;
  email?: string;
}

export interface ImportUser {
  id: string;
  name: string;
  email: string;
  nim: string | null;
}

type Match =
  | { status: 'email' | 'name'; user: ImportUser }
  | { status: 'missing' | 'ambiguous' };

// Parse both candidate separators: only separators outside quotes count.
function parseDelimited(source: string, separator: string): string[][] {
  const records: string[][] = [];
  let record: string[] = [];
  let field = '';
  let quoted = false;
  let closed = false;
  const finishRecord = () => {
    record.push(field);
    if (record.length > 1 || record.some((value) => value.trim() !== '')) {
      records.push(record);
    }
    record = [];
    field = '';
    closed = false;
  };
  for (let i = 0; i < source.length; i++) {
    const char = source[i];
    if (quoted) {
      if (char === '"' && source[i + 1] === '"') {
        field += '"';
        i++;
      } else if (char === '"') {
        quoted = false;
        closed = true;
      } else {
        field += char;
      }
    } else if (char === separator) {
      record.push(field);
      field = '';
      closed = false;
    } else if (char === '\n' || char === '\r') {
      if (char === '\r' && source[i + 1] === '\n') i++;
      finishRecord();
    } else if (char === '"' && field === '' && !closed) {
      quoted = true;
    } else if (closed || char === '"') {
      throw new Error('Format tanda kutip CSV tidak valid.');
    } else {
      field += char;
    }
  }
  if (quoted) throw new Error('Tanda kutip CSV belum ditutup.');
  finishRecord();
  return records;
}

export function parseCsv(csv: string): NimRow[] {
  const source = csv.replace(/^\uFEFF/, '');
  for (const separator of [',', ';']) {
    let records: string[][];
    try {
      records = parseDelimited(source, separator);
    } catch {
      continue;
    }
    const headers = records[0]?.map((header) => header.trim().toLowerCase());
    if (!headers?.includes('nama') || !headers.includes('nim')) continue;
    if (new Set(headers).size !== headers.length) {
      throw new Error('Header CSV duplikat.');
    }
    const nameIndex = headers.indexOf('nama');
    const nimIndex = headers.indexOf('nim');
    const emailIndex = headers.indexOf('email');
    return records.slice(1).map((record, index) => {
      if (record.length !== headers.length) {
        throw new Error(
          `Jumlah kolom CSV tidak sesuai pada baris data ${index + 1}.`,
        );
      }
      return {
        nama: record[nameIndex].trim(),
        nim: record[nimIndex],
        ...(emailIndex >= 0 ? { email: record[emailIndex].trim() } : {}),
      };
    });
  }
  throw new Error('CSV tidak valid: wajib memiliki header nama dan nim.');
}

export function normalizeNim(value: string): string | null {
  const nim = value.replace(/\s/g, '');
  return nim && /^[a-zA-Z0-9.-]+$/.test(nim) ? nim : null;
}

export function matchUser(row: NimRow, users: ImportUser[]): Match {
  if (row.email?.trim()) {
    const matches = users.filter(
      (user) => user.email.toLowerCase() === row.email!.trim().toLowerCase(),
    );
    if (matches.length > 1) return { status: 'ambiguous' };
    if (matches.length === 1) return { status: 'email', user: matches[0] };
  }
  const name = row.nama.trim().toLowerCase();
  const matches = name
    ? users.filter((user) => user.name.toLowerCase() === name)
    : [];
  if (matches.length > 1) return { status: 'ambiguous' };
  if (matches.length === 1) return { status: 'name', user: matches[0] };
  return { status: 'missing' };
}

export function planImport(rows: NimRow[], users: ImportUser[]) {
  const entries = rows.map((row, index) => ({
    row: index + 1,
    nama: row.nama,
    nim: normalizeNim(row.nim),
    match: matchUser(row, users),
  }));
  const targets = new Map<string, Set<string>>();
  for (const entry of entries) {
    if ('user' in entry.match && entry.nim !== null) {
      const values = targets.get(entry.match.user.id) ?? new Set<string>();
      values.add(entry.nim);
      targets.set(entry.match.user.id, values);
    }
  }
  const changes = new Map<string, string>();
  const details = entries.map((entry) => {
    const { match, nim } = entry;
    const conflict =
      'user' in match && (targets.get(match.user.id)?.size ?? 0) > 1;
    const status = conflict ? 'ambiguous' : match.status;
    const changed =
      !conflict && 'user' in match && nim !== null && match.user.nim !== nim;
    if (changed && 'user' in match) changes.set(match.user.id, nim);
    return {
      baris: entry.row,
      nama: entry.nama,
      akun: 'user' in match ? match.user.id : null,
      status,
      nimSebelum: 'user' in match ? match.user.nim : null,
      nimSesudah: nim,
      nimValid: nim !== null,
      berubah: changed,
    };
  });
  const count = (status: string) =>
    details.filter((entry) => entry.status === status).length;
  return {
    details,
    changes,
    summary: {
      jumlahBaris: rows.length,
      cocokViaEmail: count('email'),
      cocokViaNama: count('name'),
      tidakDitemukan: count('missing'),
      ambigu: count('ambiguous'),
      nimTidakValid: details.filter((entry) => !entry.nimValid).length,
      barisAkanBerubah: details.filter((entry) => entry.berubah).length,
      barisTidakBerubah: details.filter((entry) => !entry.berubah).length,
      akunAkanBerubah: changes.size,
    },
  };
}

export async function importNim(
  dataSource: DataSource,
  rows: NimRow[],
  apply = false,
) {
  const execute = async (manager: EntityManager) => {
    const users = await manager.query<ImportUser[]>(
      `SELECT id, name, email, nim FROM users${apply ? ' FOR UPDATE' : ''}`,
    );
    const plan = planImport(rows, users);
    console.log(
      apply ? 'Pratinjau sebelum penyimpanan' : 'Pratinjau (tanpa update)',
    );
    console.table(plan.details);
    console.log(plan.summary);
    if (apply) {
      if (plan.summary.ambigu || plan.summary.tidakDitemukan) {
        throw new Error(
          'Impor dibatalkan: ada baris ambigu atau tidak ditemukan.',
        );
      }
      // Raw SQL deliberately avoids TypeORM's automatic updatedAt mutation.
      for (const [id, nim] of plan.changes) {
        await manager.query('UPDATE users SET nim = $1 WHERE id = $2', [
          nim,
          id,
        ]);
      }
    }
    return plan.summary;
  };
  const summary = apply
    ? await dataSource.transaction(execute)
    : await execute(dataSource.manager);
  if (apply)
    console.log('Impor berhasil:', { akunDiperbarui: summary.akunAkanBerubah });
  return summary;
}

export function parseArguments(args: string[]) {
  const paths = args.filter((arg) => !arg.startsWith('--'));
  if (
    paths.length !== 1 ||
    args.some((arg) => arg.startsWith('--') && arg !== '--apply') ||
    args.filter((arg) => arg === '--apply').length > 1
  ) {
    throw new Error(
      'Cara pakai: node dist/src/scripts/import-nim.js <path-csv> [--apply]',
    );
  }
  return { path: paths[0], apply: args.includes('--apply') };
}

if (require.main === module) {
  void (async () => {
    const { path, apply } = parseArguments(process.argv.slice(2));
    const rows = parseCsv(await readFile(path, 'utf8'));
    await runSeed(async (dataSource) => {
      await importNim(dataSource, rows, apply);
    });
  })().catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}
