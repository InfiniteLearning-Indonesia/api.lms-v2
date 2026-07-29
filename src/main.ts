import { NestFactory } from '@nestjs/core';
import { json, urlencoded } from 'express';
import { ValidationPipe } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import session from 'express-session';
import passport from 'passport';
import { AppModule } from './app.module.js';
import { DataSource } from 'typeorm';
import { User, UserRole, UserStatus } from './users/entities/user.entity.js';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  app.use(json({ limit: '5mb' }));
  app.use(urlencoded({ limit: '5mb', extended: true }));

  const configService = app.get(ConfigService);
  const port = configService.get<number>('PORT', 7000);
  const sessionSecret = configService.get<string>('SESSION_SECRET', 'change-me');
  const frontendUrl = configService.get<string>('FRONTEND_URL', 'http://localhost:3000').replace(/\/$/, '');
  const isProduction = configService.get<string>('NODE_ENV') === 'production';

  // Bootstrap: ensure bootstrap admin exists (idempotent upsert)
  const dataSource = app.get(DataSource);
  const userRepo = dataSource.getRepository(User);
  const adminEmail = configService.get<string>('ADMIN_EMAIL', 'arifiansaputra43@gmail.com');
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
    await dataSource.query(`ALTER TYPE users_status_enum ADD VALUE IF NOT EXISTS 'graduated'`);
    console.log(`✅ Bootstrap: Postgres Enum 'users_status_enum' updated with 'graduated'.`);
  } catch (err: any) {
    console.warn(`⚠️ Bootstrap: Enum update note:`, err?.message);
  }

  // Validation pipe
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true,
      transform: true,
    }),
  );

  // Trust proxy for reverse proxy setup (Nginx/Cloudflare)
  // Trust proxy for reverse proxy setup (Nginx/Cloudflare)
  const expressApp = app.getHttpAdapter().getInstance();
  expressApp.set('trust proxy', true);

  // CORS config: Bulletproof setup that accepts any frontend origin with credentials
  app.enableCors({
    origin: (origin, callback) => {
      // Reflect the request origin so credentials (session cookies) work seamlessly from any domain
      callback(null, true);
    },
    credentials: true,
    methods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD'],
    allowedHeaders: ['Content-Type', 'Authorization', 'Accept', 'X-Requested-With', 'Cookie'],
  });

  // Session middleware
  const isHttps = isProduction || frontendUrl.startsWith('https://');
  const isInfiniteDomain = frontendUrl.includes('infinitelearningstudent.id') || isProduction;
  const cookieDomain = isInfiniteDomain ? '.infinitelearningstudent.id' : undefined;

  app.use(
    session({
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
