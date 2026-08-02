import { NestFactory } from '@nestjs/core';
import { json, urlencoded } from 'express';
import { ValidationPipe } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import session from 'express-session';
import connectPgSimple from 'connect-pg-simple';
import pg from 'pg';
import passport from 'passport';
import helmet from 'helmet';
import { AppModule } from './app.module.js';
import { DataSource } from 'typeorm';
import { User, UserRole, UserStatus } from './users/entities/user.entity.js';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  // Security Hardening: HTTP headers
  app.use(helmet({ crossOriginResourcePolicy: { policy: "cross-origin" } })); // Adjust if CORS issues happen

  app.use(json({ limit: '5mb' }));
  app.use(urlencoded({ limit: '5mb', extended: true }));

  const configService = app.get(ConfigService);
  const port = configService.get<number>('PORT', 7000);
  const sessionSecret = configService.get<string>(
    'SESSION_SECRET',
    'change-me',
  );
  const frontendUrl = configService
    .get<string>('FRONTEND_URL', 'http://localhost:3000')
    .replace(/\/$/, '');
  const isProduction = configService.get<string>('NODE_ENV') === 'production';

  // Bootstrap: ensure bootstrap admin exists (idempotent upsert)
  const dataSource = app.get(DataSource);
  const userRepo = dataSource.getRepository(User);
  const adminEmail = configService.get<string>(
    'ADMIN_EMAIL',
    'arifiansaputra43@gmail.com',
  );
  const adminName = configService.get<string>('ADMIN_NAME', 'Arifian Saputra');
  const existing = await userRepo.findOne({ where: { email: adminEmail } });
  if (!existing) {
    await userRepo.save(
      userRepo.create({
        email: adminEmail,
        name: adminName,
        roles: [UserRole.ADMIN],
        status: UserStatus.ACTIVE,
      }),
    );
    console.log(`✅ Bootstrap: admin "${adminEmail}" created.`);
  }

  // Ensure PostgreSQL enum users_status_enum includes 'graduated'
  try {
    await dataSource.query(
      `ALTER TYPE users_status_enum ADD VALUE IF NOT EXISTS 'graduated'`,
    );
    await dataSource.query(
      `ALTER TABLE "classes" ADD COLUMN IF NOT EXISTS "isTranscriptReleased" boolean DEFAULT false`,
    );
    console.log(
      `✅ Bootstrap: Postgres Enum & 'classes.isTranscriptReleased' column updated.`,
    );
  } catch (err: any) {
    console.warn(`⚠️ Bootstrap: Schema update note:`, err?.message);
  }

  // Ensure session table exists for connect-pg-simple
  try {
    await dataSource.query(`
      CREATE TABLE IF NOT EXISTS "session" (
        "sid" varchar NOT NULL COLLATE "default",
        "sess" json NOT NULL,
        "expire" timestamp(6) NOT NULL,
        CONSTRAINT "session_pkey" PRIMARY KEY ("sid") NOT DEFERRABLE INITIALLY IMMEDIATE
      ) WITH (OIDS=FALSE);
    `);
    await dataSource.query(`
      CREATE INDEX IF NOT EXISTS "IDX_session_expire" ON "session" ("expire");
    `);
    console.log(`✅ Bootstrap: Postgres 'session' table verified.`);
  } catch (err: any) {
    console.warn(`⚠️ Bootstrap: Session table update note:`, err?.message);
  }

  // Validation pipe (Mitigates Mass Assignment)
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true, // Strips non-DTO properties
      forbidNonWhitelisted: false, // Don't throw errors to avoid breaking legacy endpoints, just strip.
      transform: true,
    }),
  );

  // Trust proxy for reverse proxy setup (Nginx/Cloudflare)
  const expressApp = app.getHttpAdapter().getInstance();
  expressApp.set('trust proxy', true);
  // Security: hide framework fingerprint
  expressApp.disable('x-powered-by');

  // CORS config: Bulletproof setup that accepts only trusted origins
  app.enableCors({
    origin: (origin, callback) => {
      const allowedOrigins = [
        frontendUrl,
        'https://dev-lms-v2.infinitelearningstudent.id',
        'https://lms-v2.infinitelearningstudent.id',
        'http://localhost:3000',
        'http://localhost:3001',
      ].filter(Boolean);
      // Allow requests with no origin (e.g. mobile apps, Postman, server-to-server)
      if (!origin || allowedOrigins.includes(origin)) {
        callback(null, true);
      } else {
        callback(new Error('CORS: Origin not allowed'), false);
      }
    },
    credentials: true,
    methods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD'],
    allowedHeaders: [
      'Content-Type',
      'Authorization',
      'Accept',
      'X-Requested-With',
      'Cookie',
    ],
  });

  // Session middleware
  const isHttps = isProduction || frontendUrl.startsWith('https://');
  const isInfiniteDomain =
    frontendUrl.includes('infinitelearningstudent.id') || isProduction;
  const cookieDomain = isInfiniteDomain
    ? '.infinitelearningstudent.id'
    : undefined;

  const pgPool = new pg.Pool({
    host: configService.get<string>('DB_HOST', 'localhost'),
    port: Number(configService.get<string>('DB_PORT', '5432')),
    user:
      configService.get<string>('DB_USER') ||
      configService.get<string>('DB_USERNAME') ||
      'postgres',
    password: configService.get<string>('DB_PASSWORD', 'postgres'),
    database:
      configService.get<string>('DB_NAME') ||
      configService.get<string>('DB_DATABASE') ||
      'postgres',
    ssl:
      configService.get<string>('DB_SSL') === 'true' ||
      configService.get<string>('DB_HOST', '').includes('supabase')
        ? { rejectUnauthorized: false }
        : false,
  });
  const PgSession = connectPgSimple(session);

  app.use(
    session({
      store: new PgSession({
        pool: pgPool,
        tableName: 'session',
        createTableIfMissing: false,
      }),
      secret: sessionSecret,
      resave: false,
      saveUninitialized: false,
      proxy: true,
      cookie: {
        path: '/',
        domain: cookieDomain,
        httpOnly: true,
        secure: isHttps,
        sameSite: isHttps ? 'none' : 'lax',
        maxAge: 24 * 60 * 60 * 1000, // 24 hours
      },
    }),
  );

  // Passport middlewares
  app.use(passport.initialize());
  app.use(passport.session());

  await app.listen(port, '0.0.0.0');
  console.log(`🚀 Backend server is running on http://0.0.0.0:${port}`);
}
bootstrap();
