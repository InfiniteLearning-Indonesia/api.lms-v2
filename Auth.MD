# Authentication Implementation Plan (Core)
## Version 1.0 — NestJS

---

# Objective

Membangun sistem autentikasi internal untuk LMS menggunakan Google OAuth dengan mekanisme Invitation, dibangun di atas **NestJS**.

Tech stack backend:

- NestJS (framework utama)
- TypeORM (atau Prisma) untuk database
- Passport (`passport-google-oauth20`) untuk strategi OAuth
- `express-session` untuk session, karena NestJS berjalan di atas Express platform secara default

Sistem harus memenuhi kebutuhan berikut:

- Tidak menggunakan password.
- Login hanya menggunakan Google.
- Hanya user yang telah didaftarkan Admin yang dapat login.
- Mendukung tiga role:
  - Admin
  - Mentor
  - Student
- Mudah dikembangkan ke fitur lain tanpa mengubah fondasi autentikasi.
- Memanfaatkan modularitas dan Dependency Injection NestJS agar auth logic terisolasi rapi dari fitur lain.

---

# Authentication Concept

Authentication dan Authorization dipisahkan.

Google hanya digunakan untuk memverifikasi identitas pengguna.

Backend menentukan apakah pengguna diperbolehkan masuk ke sistem.

```
Google
↓
Siapa kamu?
↓
Backend (NestJS)
↓
Apakah kamu terdaftar?
↓
Ya
↓
Masuk
```

Langkah "Siapa kamu?" ditangani oleh **PassportStrategy** (`GoogleStrategy`). Langkah "Apakah kamu terdaftar?" ditangani oleh **AuthService**, yang memanggil **UsersService**.

---

# Login Flow

## Step 1

Admin membuat akun user lewat endpoint yang dilindungi guard khusus Admin.

```
Admin Dashboard
↓
Users
↓
Create User (POST /users)
```

Input divalidasi lewat DTO:

```ts
// users/dto/create-user.dto.ts
import { IsEmail, IsString, IsEnum } from 'class-validator';
import { UserRole } from '../entities/user.entity';

export class CreateUserDto {
  @IsString()
  name: string;

  @IsEmail()
  email: string;

  @IsEnum(UserRole)
  role: UserRole;
}
```

`UsersService` membuat data user dengan:

```
status = invited
```

Belum ada password.

Belum ada Google ID.

---

## Step 2

User membuka halaman Login.

```
LMS
↓
Continue with Google
```

Frontend mengarahkan browser ke:

```
GET /auth/google
```

---

## Step 3

Google Authentication

Endpoint `/auth/google` di-handle oleh `GoogleAuthGuard` (bawaan `@nestjs/passport`), yang otomatis me-redirect ke Google.

Google meminta user memilih akun. Misalnya:

```
student@gmail.com
```

Google mengirimkan `email`, `name`, `picture`, `sub` ke callback URL. `GoogleStrategy` menangkapnya dan meneruskannya sebagai `req.user` ke controller.

```ts
// auth/strategies/google.strategy.ts
import { PassportStrategy } from '@nestjs/passport';
import { Strategy, VerifyCallback } from 'passport-google-oauth20';
import { Injectable } from '@nestjs/common';

@Injectable()
export class GoogleStrategy extends PassportStrategy(Strategy, 'google') {
  constructor() {
    super({
      clientID: process.env.GOOGLE_CLIENT_ID,
      clientSecret: process.env.GOOGLE_CLIENT_SECRET,
      callbackURL: process.env.GOOGLE_CALLBACK_URL,
      scope: ['email', 'profile'],
    });
  }

  async validate(
    accessToken: string,
    refreshToken: string,
    profile: any,
    done: VerifyCallback,
  ) {
    const { emails, displayName, photos, id } = profile;
    const googleUser = {
      email: emails[0].value,
      name: displayName,
      avatarUrl: photos?.[0]?.value,
      googleId: id,
    };
    done(null, googleUser);
  }
}
```

---

## Step 4

Backend melakukan validasi lewat `AuthService`, yang memanggil `UsersService`.

```
SELECT * FROM users WHERE email = ?
```

```ts
// auth/auth.service.ts
async validateUser(googleUser: GoogleUserPayload) {
  const user = await this.usersService.findByEmail(googleUser.email);

  if (!user) {
    throw new ForbiddenException('Your account is not registered.');
  }

  if (user.status === 'suspended') {
    throw new ForbiddenException('Your account has been suspended.');
  }

  return user;
}
```

---

### Jika tidak ditemukan

```
403 Forbidden
Your account is not registered.
```

Di-throw sebagai `ForbiddenException`, otomatis ditangkap NestJS dan dikembalikan sebagai JSON response standar.

---

### Jika ditemukan

Lanjut login.

---

## Step 5

Jika login pertama (`google_id == NULL`), `AuthService` menyimpan:

```
google_id
last_login_at
status = active
```

---

## Step 6

Controller callback membuat session lewat `req.session`.

```ts
// auth/auth.controller.ts
@Get('google/callback')
@UseGuards(GoogleAuthGuard)
async googleCallback(@Req() req, @Res() res: Response) {
  const user = await this.authService.validateUser(req.user);
  req.session.userId = user.id;

  return res.redirect(process.env.FRONTEND_DASHBOARD_URL);
}
```

Session berhasil. User diarahkan ke Dashboard.

---

# User Status

## invited

Sudah dibuat Admin.

Belum pernah login.

---

## active

Sudah login.

Dapat menggunakan sistem.

---

## suspended

Tidak dapat login.

---

# Roles

## Admin

Mengelola sistem.

---

## Mentor

Mengelola kelas.

---

## Student

Mengikuti kelas.

---

# Database

Entity didefinisikan lewat TypeORM.

```ts
// users/entities/user.entity.ts
import { Entity, Column, PrimaryGeneratedColumn, CreateDateColumn, UpdateDateColumn } from 'typeorm';

export enum UserRole {
  ADMIN = 'admin',
  MENTOR = 'mentor',
  STUDENT = 'student',
}

export enum UserStatus {
  INVITED = 'invited',
  ACTIVE = 'active',
  SUSPENDED = 'suspended',
}

@Entity('users')
export class User {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column({ unique: true })
  email: string;

  @Column()
  name: string;

  @Column({ nullable: true })
  googleId: string;

  @Column({ nullable: true })
  avatarUrl: string;

  @Column({ type: 'enum', enum: UserRole })
  role: UserRole;

  @Column({ type: 'enum', enum: UserStatus, default: UserStatus.INVITED })
  status: UserStatus;

  @Column({ nullable: true })
  lastLoginAt: Date;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
```

---

# Authentication Module

```
src
└── auth
    ├── auth.module.ts
    ├── auth.controller.ts
    ├── auth.service.ts
    ├── dto/
    │   └── auth-response.dto.ts
    ├── strategies/
    │   └── google.strategy.ts
    ├── guards/
    │   ├── google-auth.guard.ts
    │   ├── session-auth.guard.ts
    │   └── roles.guard.ts
    └── decorators/
        ├── roles.decorator.ts
        └── current-user.decorator.ts
```

```ts
// auth/auth.module.ts
import { Module } from '@nestjs/common';
import { PassportModule } from '@nestjs/passport';
import { AuthController } from './auth.controller';
import { AuthService } from './auth.service';
import { GoogleStrategy } from './strategies/google.strategy';
import { UsersModule } from '../users/users.module';

@Module({
  imports: [PassportModule.register({ session: true }), UsersModule],
  controllers: [AuthController],
  providers: [AuthService, GoogleStrategy],
  exports: [AuthService],
})
export class AuthModule {}
```

---

# User Module

```
src
└── users
    ├── users.module.ts
    ├── users.controller.ts
    ├── users.service.ts
    ├── entities/
    │   └── user.entity.ts
    └── dto/
        ├── create-user.dto.ts
        └── update-user.dto.ts
```

```ts
// users/users.module.ts
import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { User } from './entities/user.entity';
import { UsersController } from './users.controller';
import { UsersService } from './users.service';

@Module({
  imports: [TypeOrmModule.forFeature([User])],
  controllers: [UsersController],
  providers: [UsersService],
  exports: [UsersService],
})
export class UsersModule {}
```

---

# API

## Login

```
GET /auth/google
```

```ts
@Get('google')
@UseGuards(GoogleAuthGuard)
async googleLogin() {
  // Guard otomatis redirect ke Google
}
```

---

## Callback

```
GET /auth/google/callback
```

Google mengembalikan user. Backend memvalidasi (lihat Step 6 di atas).

---

## Current User

```
GET /auth/me
```

```ts
@Get('me')
@UseGuards(SessionAuthGuard)
getCurrentUser(@CurrentUser() user: User) {
  return user;
}
```

---

## Logout

```
POST /auth/logout
```

```ts
@Post('logout')
@UseGuards(SessionAuthGuard)
logout(@Req() req: Request) {
  req.session.destroy(() => {});
  return { message: 'Logged out' };
}
```

---

# Session

Menggunakan HTTP Only Cookie. `express-session` dipasang di `main.ts`:

```ts
// main.ts
app.use(
  session({
    secret: process.env.SESSION_SECRET,
    resave: false,
    saveUninitialized: false,
    cookie: { httpOnly: true, secure: process.env.NODE_ENV === 'production' },
  }),
);
app.use(passport.initialize());
app.use(passport.session());
```

Cookie berisi session yang valid.

Frontend tidak menyimpan access token.

---

# Guard

Guard adalah class yang menentukan boleh/tidaknya sebuah request diteruskan ke handler controller, dieksekusi sebelum method controller dipanggil.

## SessionAuthGuard

Memastikan user sudah login.

```ts
// auth/guards/session-auth.guard.ts
import { CanActivate, ExecutionContext, Injectable, UnauthorizedException } from '@nestjs/common';

@Injectable()
export class SessionAuthGuard implements CanActivate {
  canActivate(context: ExecutionContext): boolean {
    const request = context.switchToHttp().getRequest();
    if (!request.session?.userId) {
      throw new UnauthorizedException();
    }
    return true;
  }
}
```

Jika belum login:

```
401 Unauthorized
```

---

## RolesGuard

Memastikan role sesuai, dipasangkan dengan decorator kustom `@Roles()`.

```ts
// auth/decorators/roles.decorator.ts
import { SetMetadata } from '@nestjs/common';
export const Roles = (...roles: string[]) => SetMetadata('roles', roles);
```

```ts
// auth/guards/roles.guard.ts
import { CanActivate, ExecutionContext, Injectable, ForbiddenException } from '@nestjs/common';
import { Reflector } from '@nestjs/core';

@Injectable()
export class RolesGuard implements CanActivate {
  constructor(private reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    const requiredRoles = this.reflector.get<string[]>('roles', context.getHandler());
    if (!requiredRoles) return true;

    const { user } = context.switchToHttp().getRequest();
    if (!requiredRoles.includes(user?.role)) {
      throw new ForbiddenException();
    }
    return true;
  }
}
```

Pemakaian di controller:

```ts
@Post()
@UseGuards(SessionAuthGuard, RolesGuard)
@Roles('admin')
createUser(@Body() dto: CreateUserDto) {
  return this.usersService.create(dto);
}
```

Jika bukan Admin:

```
403 Forbidden
```

---

# Invitation Flow

```
Admin
↓
Create User (POST /users, dilindungi RolesGuard('admin'))
↓
Database
↓
status = invited
↓
User Login Google
↓
Email ditemukan
↓
status = active
↓
Dashboard
```

---

# Failed Login

Semua kondisi berikut di-throw sebagai built-in HTTP Exception; NestJS Exception Filter otomatis mengubahnya jadi JSON response dengan status code yang sesuai.

## Email belum terdaftar

```ts
throw new ForbiddenException('Your account is not registered.');
```

```
403 Forbidden
Your account is not registered.
```

---

## Akun suspended

```ts
throw new ForbiddenException('Your account has been suspended.');
```

```
403 Forbidden
Your account has been suspended.
```

---

## Google Authentication gagal

```
401 Unauthorized
```

Ditangani otomatis oleh `GoogleAuthGuard` ketika Passport strategy gagal memverifikasi.

---

# Security Rules

- Tidak ada password.
- Email Google harus sudah verified.
- Email harus unik (`@Column({ unique: true })` di entity).
- Google ID hanya dapat diikat satu kali.
- Semua endpoint selain login wajib melalui `SessionAuthGuard`.
- Semua endpoint Admin wajib melalui `RolesGuard` + `@Roles('admin')`.
- Semua input wajib divalidasi lewat DTO + global `ValidationPipe` (`app.useGlobalPipes(new ValidationPipe())` di `main.ts`).

---

# Initial Milestone

## Sprint 0 — Project Setup

- Inisialisasi project dengan `@nestjs/cli` (`nest new`)
- Install `@nestjs/passport`, `passport`, `passport-google-oauth20`, `express-session`
- Install `@nestjs/typeorm` + driver database (atau setup Prisma)
- Konfigurasi `ValidationPipe` global dan `class-validator`/`class-transformer`
- Setup environment variables (`GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `SESSION_SECRET`, dll)

## Sprint 1 — Core Auth

- Database Users (Entity + Migration)
- Google OAuth (Strategy + Guard)
- Login
- Logout
- Session
- SessionAuthGuard

## Sprint 2 — User Management & RBAC

- Create User (dengan DTO validation)
- Invitation Flow
- User Management Module
- RolesGuard + `@Roles()` decorator
- Protected Routes

---

# Success Criteria

- Admin dapat membuat user.
- User yang belum didaftarkan tidak dapat login.
- User yang sudah diundang dapat login menggunakan Google.
- Role berhasil dikenali lewat `RolesGuard`.
- Session tersimpan dengan aman lewat HttpOnly cookie.
- Seluruh endpoint private terlindungi oleh `SessionAuthGuard`.