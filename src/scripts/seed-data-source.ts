import 'reflect-metadata';
import { ConfigModule, ConfigService } from '@nestjs/config';
import { join } from 'node:path';
import { DataSource } from 'typeorm';

// Manual seeds use the staging config without starting Nest or synchronizing schema.
export async function runSeed(seed: (dataSource: DataSource) => Promise<void>) {
  await ConfigModule.forRoot({
    envFilePath: ['.env.staging', '.env.local', '.env'],
  });
  const config = new ConfigService();
  if (config.get<string>('NODE_ENV') === 'production') {
    throw new Error('Manual transcript seeds cannot run in production.');
  }
  const host = config.get<string>('DB_HOST', 'localhost');
  const dataSource = new DataSource({
    type: 'postgres',
    host,
    port: Number(config.get<string>('DB_PORT', '5432')),
    username:
      config.get<string>('DB_USER') ||
      config.get<string>('DB_USERNAME') ||
      'postgres',
    password: config.get<string>('DB_PASSWORD', 'postgres'),
    database:
      config.get<string>('DB_NAME') ||
      config.get<string>('DB_DATABASE') ||
      'postgres',
    ssl:
      config.get<string>('DB_SSL') === 'true' || host.includes('supabase')
        ? { rejectUnauthorized: false }
        : false,
    entities: [join(__dirname, '../**/*.entity{.ts,.js}')],
    synchronize: false,
  });
  await dataSource.initialize();
  try {
    await seed(dataSource);
  } finally {
    await dataSource.destroy();
  }
}
