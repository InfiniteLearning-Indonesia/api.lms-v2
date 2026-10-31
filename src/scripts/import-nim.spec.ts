import { DataSource } from 'typeorm';
import {
  importNim,
  matchUser,
  normalizeNim,
  parseArguments,
  parseCsv,
  planImport,
} from './import-nim';
import type { ImportUser } from './import-nim';

const users: ImportUser[] = [
  { id: '1', name: 'Ayu', email: 'ayu@example.com', nim: '001' },
  { id: '2', name: 'Budi', email: 'budi@example.com', nim: null },
  { id: '3', name: 'Budi', email: 'other@example.com', nim: null },
];

describe('CSV parser', () => {
  it('parses commas, mixed case headers, BOM, CRLF and optional email', () => {
    expect(
      parseCsv('\uFEFFNaMa,NiM,EMAIL\r\nAyu,001,ayu@example.com\r\n'),
    ).toEqual([{ nama: 'Ayu', nim: '001', email: 'ayu@example.com' }]);
    expect(parseCsv('nama,nim\nAyu,001')).toEqual([
      { nama: 'Ayu', nim: '001' },
    ]);
  });

  it('parses semicolons, quoted separators, escaped quotes and multiline fields', () => {
    expect(parseCsv('"NAMA";"NIM"\n"Ayu; Putri, ""A""\nDewi";"00 1"')).toEqual([
      { nama: 'Ayu; Putri, "A"\nDewi', nim: '00 1' },
    ]);
    expect(parseCsv('nama,nim\n"Ayu, Putri",001')[0].nama).toBe('Ayu, Putri');
  });

  it.each([
    'nama,email\nAyu,a@example.com',
    'nama,nim\n"Ayu,001',
    'nama,nim\nAyu,001,extra',
    'nama,nim,NIM\nAyu,001,002',
    'nama,nim\n"Ayu"oops,001',
  ])('rejects malformed CSV: %s', (csv) => {
    expect(() => parseCsv(csv)).toThrow();
  });

  it('ignores blank lines and retains empty NIM for validation', () => {
    expect(parseCsv('nama;nim\n\nAyu;\n')).toEqual([{ nama: 'Ayu', nim: '' }]);
  });
});

describe('NIM normalization', () => {
  it('removes whitespace and preserves letters, leading zeros, dots and hyphens', () => {
    expect(normalizeNim(' 00 A\t1.2-3\n')).toBe('00A1.2-3');
  });
  it.each(['', ' \t\n', '12/34', '12_34', '12@34', 'é123'])(
    'rejects %j',
    (value) => {
      expect(normalizeNim(value)).toBeNull();
    },
  );
});

describe('account matching', () => {
  it('prefers case insensitive email over a different name', () => {
    expect(
      matchUser({ nama: 'Budi', email: 'AYU@EXAMPLE.COM', nim: '2' }, users),
    ).toEqual({
      status: 'email',
      user: users[0],
    });
  });
  it.each([undefined, '', 'missing@example.com'])(
    'falls back to unique exact name with email %j',
    (email) => {
      expect(matchUser({ nama: 'aYU', email, nim: '2' }, users)).toEqual({
        status: 'name',
        user: users[0],
      });
    },
  );
  it('marks duplicate names as ambiguous', () => {
    expect(matchUser({ nama: 'BUDI', nim: '2' }, users)).toEqual({
      status: 'ambiguous',
    });
  });
  it('marks duplicate case insensitive emails as ambiguous', () => {
    expect(
      matchUser({ nama: 'Ayu', email: 'ayu@example.com', nim: '2' }, [
        ...users,
        { ...users[1], email: 'AYU@example.com' },
      ]),
    ).toEqual({ status: 'ambiguous' });
  });
  it.each(['Unknown', 'Ay', ''])(
    'does not match partial or empty names: %j',
    (nama) => {
      expect(matchUser({ nama, nim: '2' }, users)).toEqual({
        status: 'missing',
      });
    },
  );
});

describe('import plan', () => {
  it('counts unchanged, changed and invalid rows and deduplicates updates', () => {
    const plan = planImport(
      [
        { nama: 'Ayu', nim: '0 01' },
        { nama: 'Budi', email: 'budi@example.com', nim: '002' },
        { nama: 'Budi', email: 'budi@example.com', nim: '002' },
        { nama: 'Ayu', nim: '/' },
      ],
      users,
    );
    expect(plan.summary).toEqual({
      jumlahBaris: 4,
      cocokViaEmail: 2,
      cocokViaNama: 2,
      tidakDitemukan: 0,
      ambigu: 0,
      nimTidakValid: 1,
      barisAkanBerubah: 2,
      barisTidakBerubah: 2,
      akunAkanBerubah: 1,
    });
    expect([...plan.changes]).toEqual([['2', '002']]);
  });
  it('rejects conflicting NIMs targeting the same account', () => {
    const plan = planImport(
      [
        { nama: 'Ayu', nim: '001' },
        { nama: 'Ayu', nim: '002' },
      ],
      users,
    );
    expect(plan.summary.ambigu).toBe(2);
    expect(plan.changes.size).toBe(0);
  });
});

describe('import execution without a database', () => {
  let dataSource: DataSource;
  let query: jest.SpyInstance;

  beforeEach(() => {
    // Constructing a DataSource does not connect; every query is mocked.
    dataSource = new DataSource({ type: 'postgres' });
    query = jest.spyOn(dataSource.manager, 'query').mockResolvedValue(users);
    jest.spyOn(console, 'log').mockImplementation(() => undefined);
    jest.spyOn(console, 'table').mockImplementation(() => undefined);
  });
  afterEach(() => jest.restoreAllMocks());

  it('only reads in preview and never opens a transaction', async () => {
    const transaction = jest.spyOn(dataSource, 'transaction');
    await importNim(dataSource, [{ nama: 'Ayu', nim: '002' }]);
    expect(query.mock.calls).toEqual([
      ['SELECT id, name, email, nim FROM users'],
    ]);
    expect(transaction).not.toHaveBeenCalled();
  });

  function mockTransaction() {
    return jest
      .spyOn(dataSource, 'transaction')
      .mockImplementation(async (isolationOrRun, run) => {
        const callback =
          typeof isolationOrRun === 'function' ? isolationOrRun : run;
        return callback(dataSource.manager);
      });
  }

  it('applies only changed valid NIMs in one transaction, with only nim in SET', async () => {
    const transaction = mockTransaction();
    await importNim(
      dataSource,
      [
        { nama: 'Ayu', nim: '002' },
        { nama: 'Budi', email: 'budi@example.com', nim: '/' },
      ],
      true,
    );
    expect(transaction).toHaveBeenCalledTimes(1);
    expect(query.mock.calls).toEqual([
      ['SELECT id, name, email, nim FROM users FOR UPDATE'],
      ['UPDATE users SET nim = $1 WHERE id = $2', ['002', '1']],
    ]);
  });

  it.each(['Budi', 'Unknown'])(
    'rejects the transaction before any writes for %s',
    async (nama) => {
      mockTransaction();
      await expect(
        importNim(
          dataSource,
          [
            { nama: 'Ayu', nim: '002' },
            { nama, nim: '003' },
          ],
          true,
        ),
      ).rejects.toThrow('Impor dibatalkan');
      expect(query).toHaveBeenCalledTimes(1);
    },
  );

  it('propagates write failures to roll back the transaction', async () => {
    mockTransaction();
    query
      .mockResolvedValueOnce(users)
      .mockRejectedValueOnce(new Error('write failed'));
    await expect(
      importNim(dataSource, [{ nama: 'Ayu', nim: '002' }], true),
    ).rejects.toThrow('write failed');
  });
});

describe('CLI arguments', () => {
  it('defaults to preview and accepts explicit apply', () => {
    expect(parseArguments(['file.csv'])).toEqual({
      path: 'file.csv',
      apply: false,
    });
    expect(parseArguments(['--apply', 'file.csv'])).toEqual({
      path: 'file.csv',
      apply: true,
    });
  });
  it.each([[], ['a', 'b'], ['a', '--unknown'], ['a', '--apply', '--apply']])(
    'rejects invalid arguments %j',
    (...args) => {
      expect(() => parseArguments(args)).toThrow('Cara pakai');
    },
  );
});
