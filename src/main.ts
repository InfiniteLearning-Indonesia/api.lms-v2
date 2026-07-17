import { NestFactory } from '@nestjs/core';
import { ValidationPipe } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import session from 'express-session';
import passport from 'passport';
import { AppModule } from './app.module.js';
import { DataSource } from 'typeorm';
import { User, UserRole, UserStatus } from './users/entities/user.entity.js';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  const configService = app.get(ConfigService);
  const port = configService.get<number>('PORT', 7000);
  const sessionSecret = configService.get<string>('SESSION_SECRET', 'change-me');
  const frontendUrl = configService.get<string>('FRONTEND_URL', 'http://localhost:3000');

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

  // Validation pipe
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true,
      transform: true,
    }),
  );

  // CORS config
  app.enableCors({
    origin: [frontendUrl],
    credentials: true,
  });

  // Session middleware
  app.use(
    session({
      secret: sessionSecret,
      resave: false,
      saveUninitialized: false,
      cookie: {
        httpOnly: true,
        secure: configService.get<string>('NODE_ENV') === 'production',
        maxAge: 24 * 60 * 60 * 1000, // 24 hours
      },
    }),
  );

  // Passport middlewares
  app.use(passport.initialize());
  app.use(passport.session());

  await app.listen(port);
  console.log(`Backend server is running on http://localhost:${port}`);
}
bootstrap();
