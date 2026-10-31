import { DataSource, ILike } from 'typeorm';
import { User } from '../users/entities/user.entity.js';
import { runSeed } from './seed-data-source.js';

const identities = [
  {
    names: ['Ardy Jabbar Hasan Harahap'],
    fullNameWithTitle: 'Ardy Jabbar Hasan Harahap, S.Tr.Kom.',
    signatureUrl: '/signatures/ardy-jabbar.png',
  },
  {
    names: ['Hafara Putri (Peja)', 'Hafara Putri'],
    fullNameWithTitle: 'Hafara Putri Fardyan Awalza, S. Ds.',
    signatureUrl: '/signatures/hafara-putri.png',
  },
  {
    names: ['Arifian Saputra'],
    email: '2001020029@student.umrah.ac.id',
    fullNameWithTitle: 'Arifian Saputra, S. T.',
    signatureUrl: '/signatures/arifian-saputra.png',
  },
  {
    names: ['Luqyana'],
    fullNameWithTitle: 'Luqyana T.',
    signatureUrl: '/signatures/luqyana.png',
  },
  {
    names: ['Riyanda Azis Febrian'],
    fullNameWithTitle: 'Riyanda Azis Febrian, S. Kom',
    signatureUrl: '/signatures/riyanda-azis.png',
  },
];

// After bootstrap DDL and build: node dist/src/scripts/seed-mentor-identity.js
export async function seedMentorIdentity(dataSource: DataSource) {
  await dataSource.transaction(async (manager) => {
    const users = manager.getRepository(User);
    for (const identity of identities) {
      const matches = await users.find({
        where: identity.names.map((name) => ({
          name: ILike(name),
          ...(identity.email ? { email: ILike(identity.email) } : {}),
        })),
      });
      if (!matches.length) {
        console.warn(`Mentor not found: ${identity.names[0]}`);
      }
      for (const user of matches) {
        if (
          user.fullNameWithTitle !== identity.fullNameWithTitle ||
          user.signatureUrl !== identity.signatureUrl
        ) {
          await users.update(user.id, {
            fullNameWithTitle: identity.fullNameWithTitle,
            signatureUrl: identity.signatureUrl,
          });
        }
      }
    }
  });
}

if (require.main === module) {
  runSeed(seedMentorIdentity).catch((error: unknown) => {
    console.error(error);
    process.exitCode = 1;
  });
}
